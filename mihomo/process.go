package mihomo

import "os/exec"

// processGuard prevents a surviving Mihomo child from retaining checker ports
// after an abrupt parent exit on the deployment platforms (Linux and Windows).
type processGuard interface {
	attach(*exec.Cmd) error
	close()
}

type noopProcessGuard struct{}

func (noopProcessGuard) attach(*exec.Cmd) error { return nil }
func (noopProcessGuard) close()                 {}
