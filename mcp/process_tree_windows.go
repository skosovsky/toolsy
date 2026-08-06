//go:build windows

package mcp

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

type processTree struct {
	job windows.Handle
}

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
}

func attachProcessTree(cmd *exec.Cmd) (processTree, error) {
	if cmd.Process == nil {
		return processTree{}, errors.New("process was not started")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processTree{}, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return processTree{}, err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		windows.CloseHandle(job)
		return processTree{}, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return processTree{}, err
	}
	status, _, _ := ntResumeProcess.Call(uintptr(process))
	if status != 0 {
		windows.CloseHandle(job)
		return processTree{}, fmt.Errorf("resume process failed with NTSTATUS %#x", status)
	}
	return processTree{job: job}, nil
}

func killProcessTree(tree processTree, cmd *exec.Cmd) error {
	if tree.job != 0 {
		return windows.CloseHandle(tree.job)
	}
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
