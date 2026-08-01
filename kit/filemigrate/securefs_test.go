package filemigrate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureFilesystemSupported_FailsClosedOnUnsafePlatforms(t *testing.T) {
	tests := []struct {
		goos string
		want bool
	}{
		{goos: "linux", want: true},
		{goos: "windows", want: true},
		{goos: "darwin", want: true},
		{goos: "js", want: false},
		{goos: "plan9", want: false},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			if got := secureFilesystemSupported(test.goos); got != test.want {
				t.Fatalf("secureFilesystemSupported(%q) = %v, want %v", test.goos, got, test.want)
			}
		})
	}
}

func TestOpenSecureFile_SymlinkSwapCannotEscapeRoot(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	victim := filepath.Join(rootDir, "migration.sql")
	outside := filepath.Join(outsideDir, "secret.sql")
	if err := os.WriteFile(victim, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	var swapErr error
	f, _, err := openSecureFile(root, "migration.sql", func() {
		if renameErr := os.Rename(victim, filepath.Join(rootDir, "original.sql")); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Symlink(outside, victim)
	})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if err == nil {
		defer func() { _ = f.Close() }()
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			t.Fatalf("unexpected successful open followed by read error: %v", readErr)
		}
		t.Fatalf("symlink swap escaped secure root and returned %q", data)
	}
	if f != nil {
		t.Fatal("failed secure open returned a file handle")
	}
}

func TestOpenSecureDir_SymlinkSwapCannotEscapeRoot(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	victim := filepath.Join(rootDir, "artifact")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "snapshot.json"), []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	var swapErr error
	child, _, err := openSecureDir(root, "artifact", func() {
		if renameErr := os.Rename(victim, filepath.Join(rootDir, "original-artifact")); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Symlink(outsideDir, victim)
	})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if err == nil {
		defer func() { _ = child.Close() }()
		t.Fatal("symlink swap opened a directory outside the secure root")
	}
	if child != nil {
		t.Fatal("failed secure directory open returned a root handle")
	}
}

func TestOpenSecureFile_IdentityReplacementRejected(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	victim := filepath.Join(rootDir, "schema.go")
	replacement := filepath.Join(rootDir, "replacement.go")
	if err := os.WriteFile(victim, []byte("package safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("package replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	f, _, err := openSecureFile(root, "schema.go", func() {
		if removeErr := os.Remove(victim); removeErr != nil {
			t.Fatalf("remove race target: %v", removeErr)
		}
		if renameErr := os.Rename(replacement, victim); renameErr != nil {
			t.Fatalf("replace race target: %v", renameErr)
		}
	})
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, errSecurePathChanged) {
		t.Fatalf("got %v, want identity-change error", err)
	}
}

func TestOpenSecureFile_SymlinkSwapToSameFileRejected(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	victim := filepath.Join(rootDir, "migration.sql")
	original := filepath.Join(rootDir, "original.sql")
	if err := os.WriteFile(victim, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	var swapErr error
	f, _, err := openSecureFile(root, "migration.sql", func() {
		if renameErr := os.Rename(victim, original); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Symlink("original.sql", victim)
	})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if f != nil {
		_ = f.Close()
		t.Fatal("same-file symlink swap returned a file handle")
	}
	if !errors.Is(err, errSecureSymlink) {
		t.Fatalf("got %v, want symlink error", err)
	}
}

func TestOpenSecureDir_SymlinkSwapToSameDirectoryRejected(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	victim := filepath.Join(rootDir, "artifact")
	original := filepath.Join(rootDir, "original-artifact")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	var swapErr error
	child, _, err := openSecureDir(root, "artifact", func() {
		if renameErr := os.Rename(victim, original); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Symlink("original-artifact", victim)
	})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if child != nil {
		_ = child.Close()
		t.Fatal("same-directory symlink swap returned a root handle")
	}
	if !errors.Is(err, errSecureSymlink) {
		t.Fatalf("got %v, want symlink error", err)
	}
}

func TestFSArtifactStore_SecureErrorEscapesControlCharacters(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}
	name := "20240101000000_bad\nname"
	if err := os.WriteFile(filepath.Join(root.RealPath, name), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = store.ReadArtifact(t.Context(), root, name, ReadArtifactOptions{})
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got %v, want invalid_path", err)
	}
	rendered := err.Error()
	if strings.ContainsAny(rendered, "\n\r\x1b") {
		t.Fatalf("rendered error contains raw control characters: %q", rendered)
	}
	if !strings.Contains(rendered, `\n`) {
		t.Fatalf("rendered error does not contain escaped migration name: %q", rendered)
	}
}

func TestSanitizeSecureFSError_RemovesPathAndPreservesClassification(t *testing.T) {
	rawPath := "untrusted\n\x1b[31mname"
	err := sanitizeSecureFSError(&os.PathError{Op: "lstat", Path: rawPath, Err: fs.ErrNotExist})
	if strings.Contains(err.Error(), rawPath) || strings.ContainsAny(err.Error(), "\n\r\x1b") {
		t.Fatalf("sanitized error contains untrusted path data: %q", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("sanitized error lost not-exist classification: %v", err)
	}
}

func TestFSStores_LstatRaceErrorsEscapeControlCharacters(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}

	assertSanitized := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("got %v, want invalid_path", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("got %v, want not-exist classification", err)
		}
		rendered := err.Error()
		if strings.ContainsAny(rendered, "\n\r\x1b") {
			t.Fatalf("rendered error contains raw control characters: %q", rendered)
		}
		if !strings.Contains(rendered, `\n`) {
			t.Fatalf("rendered error does not contain escaped entry name: %q", rendered)
		}
	}

	t.Run("artifact listing", func(t *testing.T) {
		store := NewFSArtifactStore()
		root, err := store.ResolveRoot(t.Context(), t.TempDir(), ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
		if err != nil {
			t.Fatal(err)
		}
		name := "untrusted\n\x1b[31mname"
		entryPath := filepath.Join(root.RealPath, name)
		if err := os.Mkdir(entryPath, 0o700); err != nil {
			t.Fatal(err)
		}

		var hookErr error
		hooks := secureFSTestHooks{beforeArtifactEntryLstat: func(got string) {
			if got == name {
				hookErr = os.Remove(entryPath)
			}
		}}
		ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
		_, err = store.ListArtifacts(ctx, root, ListArtifactsOptions{})
		if hookErr != nil {
			t.Fatalf("install Lstat race fixture: %v", hookErr)
		}
		assertSanitized(t, err)
	})

	t.Run("source listing", func(t *testing.T) {
		store := NewFSSourceStore()
		rootDir := t.TempDir()
		root, err := store.ResolveSourceRoot(t.Context(), rootDir)
		if err != nil {
			t.Fatal(err)
		}
		name := "untrusted\n\x1b[31mname.go"
		entryPath := filepath.Join(root.RealPath, name)
		if err := os.WriteFile(entryPath, []byte("package schema\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		var hookErr error
		hooks := secureFSTestHooks{beforeSourceEntryLstat: func(got string) {
			if got == name {
				hookErr = os.Remove(entryPath)
			}
		}}
		ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
		_, err = store.ListSourceFiles(ctx, root, ListSourceFilesOptions{})
		if hookErr != nil {
			t.Fatalf("install Lstat race fixture: %v", hookErr)
		}
		assertSanitized(t, err)
	})
}

func TestFSArtifactStore_ReadArtifact_SymlinkSwapCannotEscapeRoot(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}
	const name = "20240101000000_swap"
	if _, err := store.CreateArtifact(t.Context(), root, NewArtifact{
		Name:         name,
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{}); err != nil {
		t.Fatal(err)
	}

	victim := filepath.Join(root.RealPath, name, "migration.sql")
	outside := filepath.Join(t.TempDir(), "secret.sql")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	var swapErr error
	hooks := secureFSTestHooks{afterArtifactFileLstat: func(filename string) {
		if filename != "migration.sql" || swapErr != nil {
			return
		}
		if err := os.Rename(victim, victim+".original"); err != nil {
			swapErr = err
			return
		}
		swapErr = os.Symlink(outside, victim)
	}}
	ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
	loaded, err := store.ReadArtifact(ctx, root, name, ReadArtifactOptions{})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if loaded != nil {
		t.Fatalf("symlink swap returned artifact bytes: %q", loaded.MigrationSQL)
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got %v, want invalid_path", err)
	}
}

func TestFSArtifactStore_CreateArtifact_StagingSwapLeavesUnverifiedTarget(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	outsideSQL := filepath.Join(outsideDir, "migration.sql")
	if err := os.WriteFile(outsideSQL, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "snapshot.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var swapErr error
	hooks := secureFSTestHooks{beforeArtifactPublish: func(stagingName string) {
		stagingPath := filepath.Join(root.RealPath, stagingName)
		if err := os.Rename(stagingPath, stagingPath+"-original"); err != nil {
			swapErr = err
			return
		}
		swapErr = os.Symlink(outsideDir, stagingPath)
	}}
	ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
	const name = "20240101000000_publish_swap"
	loaded, err := store.CreateArtifact(ctx, root, NewArtifact{
		Name:         name,
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if loaded != nil {
		t.Fatalf("staging swap returned a loaded artifact: %+v", loaded)
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got %v, want invalid_path", err)
	}
	target := filepath.Join(root.RealPath, name)
	targetInfo, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatalf("unverified replacement was unexpectedly deleted: %v", statErr)
	}
	if targetInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink replacement", targetInfo.Mode())
	}
	if _, listErr := store.ListArtifacts(t.Context(), root, ListArtifactsOptions{}); !errors.Is(listErr, ErrInvalidPath) {
		t.Fatalf("discovery accepted unverified replacement: %v", listErr)
	}
	outsideBytes, err := os.ReadFile(outsideSQL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outsideBytes, []byte("outside-secret")) {
		t.Fatalf("outside bytes changed: %q", outsideBytes)
	}
}

func TestFSArtifactStore_CreateArtifact_StagingVerificationSwapLeavesReplacement(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}

	var (
		swapErr      error
		stagingPath  string
		originalPath string
	)
	hooks := secureFSTestHooks{beforeArtifactStagingVerify: func(stagingName string) {
		stagingPath = filepath.Join(root.RealPath, stagingName)
		originalPath = stagingPath + "-original"
		if err := os.Rename(stagingPath, originalPath); err != nil {
			swapErr = err
			return
		}
		if err := os.Mkdir(stagingPath, 0o700); err != nil {
			swapErr = err
			return
		}
		swapErr = os.WriteFile(filepath.Join(stagingPath, "replacement.txt"), []byte("keep"), 0o600)
	}}
	ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
	loaded, err := store.CreateArtifact(ctx, root, NewArtifact{
		Name:         "20240101000000_staging_verify_swap",
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{})
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if loaded != nil {
		t.Fatalf("staging verification swap returned a loaded artifact: %+v", loaded)
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got %v, want invalid_path", err)
	}
	replacement, err := os.ReadFile(filepath.Join(stagingPath, "replacement.txt"))
	if err != nil {
		t.Fatalf("unverified staging replacement was unexpectedly deleted: %v", err)
	}
	if !bytes.Equal(replacement, []byte("keep")) {
		t.Fatalf("replacement bytes changed: %q", replacement)
	}
	if _, err := os.Stat(filepath.Join(originalPath, "migration.sql")); err != nil {
		t.Fatalf("original staging directory was unexpectedly removed: %v", err)
	}
}

func TestCreateSecureTempDir_FailedOpenLeavesUnverifiedReplacement(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	var swapErr error
	var swappedName string
	name, child, err := createSecureTempDir(root, ".grizzle-staging-", func(created string) {
		swappedName = created
		createdPath := filepath.Join(rootDir, created)
		if renameErr := os.Rename(createdPath, createdPath+"-original"); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Symlink(outsideDir, createdPath)
	})
	if errors.Is(swapErr, os.ErrPermission) {
		t.Skipf("symlinks are not available: %v", swapErr)
	}
	if swapErr != nil {
		t.Fatalf("install race fixture: %v", swapErr)
	}
	if child != nil {
		_ = child.Close()
		t.Fatal("swapped temp directory returned an open handle")
	}
	if err == nil {
		t.Fatal("swapped temp directory did not fail secure open")
	}
	if name != "" {
		t.Fatalf("failed createSecureTempDir returned name %q", name)
	}
	replacementInfo, statErr := os.Lstat(filepath.Join(rootDir, swappedName))
	if statErr != nil {
		t.Fatalf("unverified replacement was unexpectedly deleted: %v", statErr)
	}
	if replacementInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replacement mode = %v, want symlink", replacementInfo.Mode())
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, swappedName+"-original")); statErr != nil {
		t.Fatalf("renamed-away original was unexpectedly removed: %v", statErr)
	}
}

func TestFSArtifactStore_CreateArtifact_DuplicateCheckLstatErrorSanitized(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	root, err := store.ResolveRoot(t.Context(), t.TempDir(), ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}
	// An overlong single component passes validateArtifactName but makes the
	// duplicate-check Lstat fail with a non-ENOENT error whose raw *PathError
	// embeds the caller-controlled name, control characters included.
	name := "20240101000000_bad\n\x1b[31mname_" + strings.Repeat("a", 300)
	loaded, err := store.CreateArtifact(t.Context(), root, NewArtifact{
		Name:         name,
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{})
	if loaded != nil {
		t.Fatalf("overlong name returned a loaded artifact: %+v", loaded)
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("got %v, want invalid_path", err)
	}
	rendered := err.Error()
	if strings.ContainsAny(rendered, "\n\r\x1b") {
		t.Fatalf("rendered error contains raw control characters: %q", rendered)
	}
	if !strings.Contains(rendered, `\n`) {
		t.Fatalf("rendered error does not contain escaped migration name: %q", rendered)
	}
}

func TestFSArtifactStore_CreateArtifact_IgnoresCancellationAfterPublish(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hooks := secureFSTestHooks{beforeArtifactPublish: func(string) {
		// Cancellation fires in the publish window, so every post-publish
		// verification step observes a canceled caller context.
		cancel()
	}}
	ctx = context.WithValue(ctx, secureFSTestHooksKey{}, hooks)
	const name = "20240101000000_cancel_after_publish"
	loaded, err := store.CreateArtifact(ctx, root, NewArtifact{
		Name:         name,
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{})
	if err != nil {
		t.Fatalf("cancellation during publish window reported committed artifact as failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("committed artifact was not returned")
	}
	if !bytes.Equal(loaded.MigrationSQL, []byte("safe")) {
		t.Fatalf("loaded bytes = %q, want verified staged bytes", loaded.MigrationSQL)
	}
	published, err := os.ReadFile(filepath.Join(root.RealPath, name, "migration.sql"))
	if err != nil {
		t.Fatalf("published artifact missing after cancellation: %v", err)
	}
	if !bytes.Equal(published, []byte("safe")) {
		t.Fatalf("published bytes = %q, want staged bytes", published)
	}
}

func TestFSArtifactStore_CreateArtifact_PublishedContentSwapRejected(t *testing.T) {
	if err := ensureSecureFilesystemSupported(); err != nil {
		t.Skip(err)
	}
	store := NewFSArtifactStore()
	rootDir := t.TempDir()
	root, err := store.ResolveRoot(t.Context(), rootDir, ResolveArtifactRootOptions{Mode: RootEnsureForWrite})
	if err != nil {
		t.Fatal(err)
	}

	var tamperErr error
	hooks := secureFSTestHooks{beforeArtifactPublish: func(stagingName string) {
		tamperErr = os.WriteFile(
			filepath.Join(root.RealPath, stagingName, "migration.sql"),
			[]byte("tampered"),
			0o640,
		)
	}}
	ctx := context.WithValue(t.Context(), secureFSTestHooksKey{}, hooks)
	const name = "20240101000000_content_swap"
	loaded, err := store.CreateArtifact(ctx, root, NewArtifact{
		Name:         name,
		MigrationSQL: []byte("safe"),
		SnapshotJSON: []byte("{}"),
	}, CreateArtifactOptions{})
	if tamperErr != nil {
		t.Fatalf("install race fixture: %v", tamperErr)
	}
	if loaded != nil {
		t.Fatalf("content swap returned a loaded artifact: %+v", loaded)
	}
	if !errors.Is(err, ErrInvalidPath) || !errors.Is(err, errSecureContentChanged) {
		t.Fatalf("got %v, want invalid_path content-change error", err)
	}
	published, err := os.ReadFile(filepath.Join(root.RealPath, name, "migration.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(published, []byte("tampered")) {
		t.Fatalf("published bytes = %q, want tampered fixture", published)
	}
}
