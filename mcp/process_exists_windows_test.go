//go:build windows

package mcp

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

func processExists(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}

func newLongLivedDescendant() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=TestStdioHelperProcess", "--", "long-lived-descendant")
}
