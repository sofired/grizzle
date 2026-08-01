//go:build unix

package filemigrate

import (
	"io/fs"
	"syscall"
)

// hasExtraHardLinks reports whether fi describes a regular file with more than
// one hard link, where the platform exposes link-count metadata. The artifact
// and source store contracts require rejecting hard-linked inputs because a
// hard link lets the same bytes be aliased from outside the configured root
// without using a symlink. On platforms where Nlink is exposed (Linux, macOS,
// the BSDs), this returns true when st_nlink > 1; on other platforms a
// separate build tag returns false ("where detectable", per the spec).
//
// If fi.Sys() does not yield a *syscall.Stat_t (for example, when fi comes
// from a synthetic fs.FS in tests, such as fstest.MapFS), the function
// conservatively returns false rather than panicking. Real filesystem
// readers in this package use os.Root Lstat/Stat operations, so they populate
// *syscall.Stat_t under the unix build tag.
func hasExtraHardLinks(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return st.Nlink > 1
}
