//go:build !linux && !windows

package mihomo

import (
	"os"
	"os/exec"
)

func newProcessGuard(_ *exec.Cmd) (processGuard, error) {
	return noopProcessGuard{}, nil
}

func terminateProcess(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}
