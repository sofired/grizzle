//go:build unix

package filemigrate

import (
	"io/fs"
	"syscall"
)

// hasExtraHardLinks reports whether fi describes a regular file with more than
// one hard link, where the platform exposes link-count metadata. The artifact
// and source store contracts require rejecting hardlinked inputs because a
// hardlink lets the same bytes be aliased from outside the configured root
// without using a symlink. On platforms where Nlink is exposed (Linux, macOS,
// the BSDs), this returns true when st_nlink > 1; on other platforms a
// separate build tag returns false ("where detectable", per the spec).
func hasExtraHardLinks(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return uint64(st.Nlink) > 1
}
