//go:build !unix

package filemigrate

import "io/fs"

// hasExtraHardLinks is a no-op on platforms where fs.FileInfo.Sys() does not
// return *syscall.Stat_t (Windows, Plan 9, js/wasm). On those targets,
// st_nlink-style link-count metadata is not surfaced by the standard library,
// so hard-link detection is impossible — not discretionary. The store
// contracts in docs/spec/file-migrations-api.md and
// docs/spec/file-migrations-artifacts.md phrase hard-link rejection as "where
// detectable" precisely to allow this case; this function always returns
// false. Callers that need hard-link rejection on these platforms must
// enforce it through other means (e.g., a sandboxed filesystem provider).
func hasExtraHardLinks(_ fs.FileInfo) bool {
	return false
}
