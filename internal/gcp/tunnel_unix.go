//go:build !windows

package gcp

import (
	"os/exec"
	"syscall"
)

// tunnelProcGroup runs the proxy in its own process group so its children are
// signalled along with cloud-sql-proxy on teardown.
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
