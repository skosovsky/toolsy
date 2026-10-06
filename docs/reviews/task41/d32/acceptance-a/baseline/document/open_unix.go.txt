//go:build unix

package document

import "syscall"

// Nonblocking opens let the toolkit reject FIFOs without waiting for a writer.
const nonblockFlag = syscall.O_NONBLOCK
