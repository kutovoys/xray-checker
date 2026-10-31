//go:build linux

package mihomo

import (
	"os/exec"
	"syscall"
)

func newProcessGuard(cmd *exec.Cmd) (processGuard, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return noopProcessGuard{}, nil
}

func terminateProcess(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
