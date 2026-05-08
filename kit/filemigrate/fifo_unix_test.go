//go:build unix

package filemigrate_test

import "syscall"

// makeFIFO creates a FIFO (named pipe) at path. Unix-only; the windows
// stub returns an error so the calling test skips. Used by
// TestFSSourceStore_NonRegularNonGoSidecarIgnored to seed a non-regular
// non-.go sidecar in a schema source directory.
func makeFIFO(path string) error {
	return syscall.Mkfifo(path, 0o600)
}
