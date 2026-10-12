//go:build !windows

package aws

import (
	"os/exec"
	"syscall"
)

// tunnelProcGroup runs the session in its own process group so the
// session-manager-plugin child is signalled along with the aws CLI on teardown.
func tunnelProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// TerminateTunnel signals the tunnel's whole process group with SIGTERM.
func TerminateTunnel(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// KillTunnel signals the tunnel's whole process group with SIGKILL.
func KillTunnel(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
