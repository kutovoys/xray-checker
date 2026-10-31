//go:build windows

package mihomo

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProcessGuard struct {
	job windows.Handle
}

func newProcessGuard(_ *exec.Cmd) (processGuard, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	return &windowsProcessGuard{job: job}, nil
}

func (g *windowsProcessGuard) attach(cmd *exec.Cmd) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(g.job, process)
}

func (g *windowsProcessGuard) close() {
	if g.job != 0 {
		windows.CloseHandle(g.job)
		g.job = 0
	}
}

func terminateProcess(cmd *exec.Cmd) error {
	// Go does not support sending os.Interrupt to an arbitrary Windows process.
	return cmd.Process.Kill()
}
