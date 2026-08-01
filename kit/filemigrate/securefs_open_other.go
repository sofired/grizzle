//go:build !unix

package filemigrate

import "os"

// Non-Unix filesystem implementations do not expose Unix FIFO semantics.
// js and plan9 fail before reaching this open; os.Root supplies the supported
// platform containment boundary elsewhere.
const secureReadOpenFlags = os.O_RDONLY
