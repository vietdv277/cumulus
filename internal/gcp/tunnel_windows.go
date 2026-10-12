//go:build windows

package gcp

import (
	"os/exec"
)

// tunnelProcGroup is a no-op on Windows: there are no process groups, so
// teardown signals the proxy process itself.
func tunnelProcGroup(cmd *exec.Cmd) {}

// TerminateTunnel kills the tunnel process.
func TerminateTunnel(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// KillTunnel kills the tunnel process.
func KillTunnel(cmd *exec.Cmd) error {
	return TerminateTunnel(cmd)
}
