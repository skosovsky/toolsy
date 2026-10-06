//go:build !unix

package fstool

// The host must exclude device paths on platforms without Unix nonblocking opens.
const nonblockFlag = 0
