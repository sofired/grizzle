//go:build !unix

package filemigrate_test

import "errors"

// makeFIFO is a stub for non-unix platforms. The associated FIFO test
// calls t.Skip when this returns an error.
func makeFIFO(path string) error {
	return errors.New("FIFO creation not supported on this platform")
}
