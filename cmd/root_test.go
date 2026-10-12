package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// runCaptured executes rootCmd with the given args and returns its combined
// output, restoring the writers and the touched subcommands' SilenceUsage.
func runCaptured(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		if c, _, err := rootCmd.Find(args); err == nil {
			c.SilenceUsage = false
		}
		resetLegacyFlags()
	})

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestRuntimeErrorOmitsUsage(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	out, err := runCaptured(t, "alias", "rm", "nosuch")
	if err == nil {
		t.Fatal("expected an error removing an unknown alias")
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("runtime error printed usage:\n%s", out)
	}
	if strings.Contains(out, err.Error()) {
		t.Errorf("cobra printed the error itself; Execute prints it once:\n%s", out)
	}
}

func TestArgErrorKeepsUsage(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	out, err := runCaptured(t, "alias", "add", "onlyone")
	if err == nil {
		t.Fatal("expected an argument error")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("argument error should print usage, got:\n%s", out)
	}
}
