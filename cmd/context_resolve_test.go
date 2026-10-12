package cmd

import (
	"os"
	"testing"

	"github.com/vietdv277/cumulus/internal/config"
)

const testConfigYAML = `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
  gcp:staging:
    provider: gcp
    project: my-project
    region: asia-southeast1
aliases:
  prod: aws:prod
  stg: gcp:staging
`

// setTestConfig points XDG_CONFIG_HOME and HOME at temp dirs and writes the
// given config.yaml so tests never touch the real ~/.config or ~/.kube.
func setTestConfig(t *testing.T, content string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(config.GetCMLConfigDir(), 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(config.GetCMLConfigPath(), []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// resetLegacyFlags clears the Changed state of the persistent -p/-r flags so
// one test's cobra parse cannot leak into another.
func resetLegacyFlags() {
	for _, name := range []string{"profile", "region"} {
		if f := rootCmd.PersistentFlags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

// The lookup table for resolveContext lives in internal/config/resolve_test.go;
// these tests cover the cmd wrapper, whose extra job is rejecting legacy flags.

func TestResolveContextRejectsProfileFlag(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	// Silence cobra's own error/usage printing; we assert on the returned error.
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	t.Cleanup(func() {
		rootCmd.SilenceUsage = false
		rootCmd.SilenceErrors = false
		resetLegacyFlags()
	})

	rootCmd.SetArgs([]string{"vm", "list", "-p", "dev"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when -p is passed to a context-aware command")
	}

	// The exact guard message proves the failure happened before any AWS
	// client was built (no network call, no provider creation).
	want := "--profile is not supported by context-aware commands; use --context <name> or 'cml use <context>'"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

func TestResolveContextRejectsRegionFlag(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	t.Cleanup(func() {
		rootCmd.SilenceUsage = false
		rootCmd.SilenceErrors = false
		resetLegacyFlags()
	})

	rootCmd.SetArgs([]string{"secrets", "list", "-r", "us-west-2"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when -r is passed to a context-aware command")
	}

	want := "--region is not supported by context-aware commands; use --context <name> or 'cml use <context>'"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
