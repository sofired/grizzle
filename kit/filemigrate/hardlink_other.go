//go:build !unix

package filemigrate

import "io/fs"

// hasExtraHardLinks is a no-op on platforms that do not expose link-count
// metadata in fs.FileInfo. The spec phrases hardlink rejection as "where
// detectable", so on these platforms the check silently allows the file.
func hasExtraHardLinks(_ fs.FileInfo) bool {
	return false
}
