//go:build !unix

package document

// Host must exclude device paths on platforms without Unix nonblocking opens.
const nonblockFlag = 0
