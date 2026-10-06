//go:build unix

package fstool

import "syscall"

// Nonblocking opens reject FIFOs before waiting for a peer; regular file IO is unchanged.
const nonblockFlag = syscall.O_NONBLOCK
