package filemigrate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func TestFSArtifactStore_CreateArtifact_StagingSwapFailsClosed(t *testing.T) {
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
	if _, statErr := os.Lstat(filepath.Join(root.RealPath, name)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed publish left a target entry: %v", statErr)
	}
	outsideBytes, err := os.ReadFile(outsideSQL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outsideBytes, []byte("outside-secret")) {
		t.Fatalf("outside bytes changed: %q", outsideBytes)
	}
}
