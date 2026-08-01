//go:build unix

package filemigrate

import (
	"os"
	"syscall"
)

// O_NONBLOCK prevents a regular-file-to-FIFO swap from hanging between the
// pre-open metadata check and the post-open identity/type verification.
const secureReadOpenFlags = os.O_RDONLY | syscall.O_NONBLOCK
