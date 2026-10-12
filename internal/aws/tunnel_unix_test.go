//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStopTunnelKillsProcessGroup(t *testing.T) {
	// sh runs in its own process group and a sleep child shares that group;
	// after stopTunnel the child must be gone too, not just the parent.
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	f, err := os.Create(pidFile)
	if err != nil {
		t.Fatalf("create pid file: %v", err)
	}

	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait $!")
	cmd.Stdout = f
	if err := startTunnelProc(cmd); err != nil {
		t.Fatalf("startTunnelProc() error: %v", err)
	}
	_ = f.Close() // the child holds its own copy of the fd

	// Cleanup for the fatal paths before the teardown runs.
	defer func() {
		if cmd.Process != nil {
			_ = KillTunnel(cmd)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	var childPID int
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(pidFile); readErr == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				childPID = pid
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child pid was not printed in time")
	}

	exit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exit)
		close(done)
	}()

	stopTunnel(cmd, exit)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait watcher did not finish after stopTunnel")
	}

	// The child sleep process must be gone, not only the sh parent.
	deadline = time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(childPID, syscall.Signal(0))
		if err != nil {
			return // no such process: the whole group was killed
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d still alive after stopTunnel", childPID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
