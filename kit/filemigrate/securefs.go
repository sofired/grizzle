package filemigrate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// errSecureFilesystemUnsupported is returned before filesystem access on
// platforms where os.Root cannot provide a stable handle-relative boundary.
var errSecureFilesystemUnsupported = errors.New("secure handle-relative filesystem operations are unsupported")

var errSecurePathChanged = errors.New("filesystem entry changed during secure open")

// secureFSTestHooks permits deterministic race injection through public store
// methods without mutable package globals. The key and value are unexported,
// so production callers cannot install hooks accidentally.
type secureFSTestHooks struct {
	afterArtifactFileLstat func(name string)
	beforeArtifactPublish  func(stagingName string)
}

type secureFSTestHooksKey struct{}

// ensureSecureFilesystemSupported fails closed on platforms where os.Root is
// documented as either TOCTOU-vulnerable (js) or path-based across directory
// renames (plan9). The in-memory stores remain available on those platforms.
func ensureSecureFilesystemSupported() error {
	if !secureFilesystemSupported(runtime.GOOS) {
		return fmt.Errorf("%w on %s", errSecureFilesystemUnsupported, runtime.GOOS)
	}
	return nil
}

func secureFilesystemSupported(goos string) bool {
	return goos != "js" && goos != "plan9"
}

// openSecureRootPath opens path one directory component at a time. Every
// component is Lstat-checked, opened relative to the already-open parent, and
// identity-checked after open. This closes the validate-then-reopen window and
// rejects symlinked components without relying on path-based EvalSymlinks.
//
// When create is true, missing components are created relative to their open
// parent. created reports whether the final path was created by this call.
func openSecureRootPath(path string, create bool, perm fs.FileMode) (_ *os.Root, canonical string, created bool, retErr error) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		return nil, "", false, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", false, err
	}
	abs = filepath.Clean(abs)

	volume := filepath.VolumeName(abs)
	base := volume + string(filepath.Separator)
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return nil, "", false, err
	}

	current, err := os.OpenRoot(base)
	if err != nil {
		return nil, "", false, err
	}
	defer func() {
		if retErr != nil {
			_ = current.Close()
		}
	}()

	if rel == "." {
		return current, abs, false, nil
	}
	parts, err := securePathComponents(rel)
	if err != nil {
		return nil, "", false, err
	}
	for i, part := range parts {
		info, statErr := current.Lstat(part)
		if errors.Is(statErr, fs.ErrNotExist) {
			if !create {
				return nil, abs, false, statErr
			}
			mkdirErr := current.Mkdir(part, perm)
			if mkdirErr != nil && !errors.Is(mkdirErr, fs.ErrExist) {
				return nil, abs, false, mkdirErr
			}
			info, statErr = current.Lstat(part)
			if statErr == nil && mkdirErr == nil && i == len(parts)-1 {
				created = true
			}
		}
		if statErr != nil {
			return nil, abs, false, statErr
		}

		next, _, openErr := openSecureDirFromInfo(current, part, info, nil)
		if openErr != nil {
			return nil, abs, false, openErr
		}
		if closeErr := current.Close(); closeErr != nil {
			_ = next.Close()
			return nil, abs, false, closeErr
		}
		current = next
	}
	return current, abs, created, nil
}

// openSecureDir opens a single directory entry without following a symlink.
// afterLstat is used only by deterministic race regression tests.
func openSecureDir(parent *os.Root, name string, afterLstat func()) (*os.Root, fs.FileInfo, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	return openSecureDirFromInfo(parent, name, info, afterLstat)
}

func openSecureDirFromInfo(parent *os.Root, name string, info fs.FileInfo, afterLstat func()) (*os.Root, fs.FileInfo, error) {
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("%s: symlinks are not supported", name)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s: not a directory", name)
	}
	if afterLstat != nil {
		afterLstat()
	}

	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := child.Stat(".")
	if err != nil {
		_ = child.Close()
		return nil, nil, err
	}
	if !os.SameFile(info, openedInfo) {
		_ = child.Close()
		return nil, nil, fmt.Errorf("%s: %w", name, errSecurePathChanged)
	}
	return child, openedInfo, nil
}

// openSecureFile opens a single file entry without a validate-then-open race.
// The returned FileInfo describes the opened handle, not a stale path lookup.
// afterLstat is used only by deterministic race regression tests.
func openSecureFile(parent *os.Root, name string, afterLstat func()) (*os.File, fs.FileInfo, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("%s: symlinks are not supported", name)
	}
	// Reject FIFOs, devices, sockets, and directories before open. On Unix the
	// open itself also uses O_NONBLOCK, so a regular-to-FIFO race cannot hang
	// before the post-open type and identity checks run.
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: not a regular file", name)
	}
	if afterLstat != nil {
		afterLstat()
	}

	f, err := parent.OpenFile(name, secureReadOpenFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s: not a regular file", name)
	}
	if !os.SameFile(info, openedInfo) {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s: %w", name, errSecurePathChanged)
	}
	return f, openedInfo, nil
}

func verifySecureDirIdentity(parent *os.Root, name string, want fs.FileInfo) error {
	child, got, err := openSecureDir(parent, name, nil)
	if err != nil {
		return err
	}
	defer func() { _ = child.Close() }()
	if !os.SameFile(want, got) {
		return fmt.Errorf("%s: %w", name, errSecurePathChanged)
	}
	return nil
}

// openSecureFilePath walks parent directories handle-relatively and opens the
// final file with the same no-follow identity check.
func openSecureFilePath(root *os.Root, relpath string) (*os.File, fs.FileInfo, error) {
	parts, err := securePathComponents(relpath)
	if err != nil {
		return nil, nil, err
	}
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("path must name a file under root")
	}

	current := root
	currentOwned := false
	defer func() {
		if currentOwned {
			_ = current.Close()
		}
	}()
	for _, part := range parts[:len(parts)-1] {
		next, _, openErr := openSecureDir(current, part, nil)
		if openErr != nil {
			return nil, nil, openErr
		}
		if currentOwned {
			_ = current.Close()
		}
		current = next
		currentOwned = true
	}
	return openSecureFile(current, parts[len(parts)-1], nil)
}

func readSecureDir(root *os.Root) ([]fs.DirEntry, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadDir(-1)
}

// createSecureTempDir creates and opens an unpredictable private child
// directory beneath root without ever reconstructing an absolute child path.
func createSecureTempDir(root *os.Root, prefix string) (string, *os.Root, error) {
	var random [16]byte
	for range 100 {
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := prefix + hex.EncodeToString(random[:])
		if err := root.Mkdir(name, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return "", nil, err
		}
		child, _, err := openSecureDir(root, name, nil)
		if err != nil {
			_ = root.RemoveAll(name)
			return "", nil, err
		}
		return name, child, nil
	}
	return "", nil, fmt.Errorf("could not allocate secure temporary directory")
}

func writeSecureNewFile(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func securePathComponents(path string) ([]string, error) {
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return nil, nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes root")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid path component")
		}
	}
	return parts, nil
}
