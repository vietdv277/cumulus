package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// freePort binds an ephemeral listener, records the port and closes it.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

func TestEnsureTunnelPortFree(t *testing.T) {
	// In-use port: a bound listener must make the check fail.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	err = ensureTunnelPortFree(port)
	if err == nil {
		t.Fatalf("ensureTunnelPortFree(%d) error = nil, want already-in-use error", port)
	}
	if !strings.Contains(err.Error(), "already in use") || !strings.Contains(err.Error(), "--local-port") {
		t.Errorf("ensureTunnelPortFree(%d) error = %q, want port-in-use hint mentioning --local-port", port, err.Error())
	}

	// Free port: no listener bound, check passes.
	if err := ensureTunnelPortFree(freePort(t)); err != nil {
		t.Errorf("ensureTunnelPortFree() unexpected error: %v", err)
	}
}

func TestWaitForPortReady(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := waitForPort(ctx, port, 2*time.Second, make(chan struct{})); err != nil {
		t.Errorf("waitForPort() unexpected error: %v", err)
	}
}

func TestWaitForPortTunnelExit(t *testing.T) {
	port := freePort(t)
	closed := make(chan struct{})
	close(closed)

	err := waitForPort(context.Background(), port, time.Second, closed)
	if err == nil {
		t.Fatal("waitForPort() error = nil, want early-exit error")
	}
	if !strings.Contains(err.Error(), "exited before") {
		t.Errorf("waitForPort() error = %q, want tunnel-exit hint", err.Error())
	}
}

func TestWaitForPortTimeout(t *testing.T) {
	port := freePort(t)

	start := time.Now()
	err := waitForPort(context.Background(), port, 300*time.Millisecond, make(chan struct{}))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("waitForPort() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("waitForPort() error = %q, want timeout hint", err.Error())
	}
	if elapsed > 5*time.Second {
		t.Errorf("waitForPort() took %v, want fast timeout", elapsed)
	}
}

func TestWaitForPortContextCancelled(t *testing.T) {
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForPort(ctx, port, time.Second, make(chan struct{}))
	if err == nil {
		t.Fatal("waitForPort() error = nil, want context error")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%s", context.Canceled)) {
		t.Errorf("waitForPort() error = %q, want %v", err.Error(), context.Canceled)
	}
}
