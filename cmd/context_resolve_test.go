package cmd

import (
	"os"
	"strings"
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

const testConfigNoCurrentYAML = `contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
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

func TestResolveContext(t *testing.T) {
	tests := []struct {
		name        string
		flag        string
		configYAML  string
		wantName    string
		wantProfile string
		wantErr     string
	}{
		{
			name:        "flag with alias",
			flag:        "prod",
			configYAML:  testConfigYAML,
			wantName:    "aws:prod",
			wantProfile: "prod-sso",
		},
		{
			name:       "flag with full name",
			flag:       "gcp:staging",
			configYAML: testConfigYAML,
			wantName:   "gcp:staging",
		},
		{
			name:       "flag unknown",
			flag:       "nope",
			configYAML: testConfigYAML,
			wantErr:    `context "nope" not found`,
		},
		{
			name:        "no flag with current set",
			flag:        "",
			configYAML:  testConfigYAML,
			wantName:    "aws:prod",
			wantProfile: "prod-sso",
		},
		{
			name:       "no flag and none set",
			flag:       "",
			configYAML: testConfigNoCurrentYAML,
			wantErr:    "no context set. Use 'cml use <context>' to set one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLegacyFlags()
			setTestConfig(t, tt.configYAML)

			ctxConfig, ctxName, err := resolveContext(tt.flag)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ctxName != tt.wantName {
				t.Errorf("ctxName = %q, want %q", ctxName, tt.wantName)
			}
			if ctxConfig == nil {
				t.Fatal("ctxConfig is nil")
			}
			if ctxConfig.Profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", ctxConfig.Profile, tt.wantProfile)
			}
		})
	}
}

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
