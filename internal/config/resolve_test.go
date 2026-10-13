package config

import (
	"strings"
	"testing"
)

const resolveTestYAML = `current_context: aws:prod
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

const resolveTestNoCurrentYAML = `contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
`

// suggestTestYAML has contexts, aliases and tunnels close enough to typical
// typos to exercise the "did you mean" suffixes.
const suggestTestYAML = `current_context: aws:prod
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
tunnels:
  db-prod:
    context: aws:prod
    target: i-0db123
    remote_port: 5432
    local_port: 5432
  web-ssh:
    context: aws:prod
    target: i-0web123
    remote_port: 22
    local_port: 2222
`

func TestResolveContextSuggestions(t *testing.T) {
	t.Run("flag typo suggests contexts and aliases", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		_, _, err := ResolveContext("prdo")
		if err == nil {
			t.Fatal("expected error for unknown context flag")
		}
		want := `context "prdo" not found — did you mean "aws:prod" or "prod"? — run 'cml contexts' to list configured contexts`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("flag typo without close match keeps today's text", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		_, _, err := ResolveContext("zzzzzz")
		if err == nil {
			t.Fatal("expected error for unknown context flag")
		}
		want := `context "zzzzzz" not found — run 'cml contexts' to list configured contexts`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("stale current context suggests", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), `current_context: aws:prd
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
`, 0600)

		_, _, err := ResolveContext("")
		if err == nil {
			t.Fatal("expected error for a stale current context")
		}
		want := `context "aws:prd" not found — did you mean "aws:prod"? — run 'cml contexts' to list configured contexts`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})
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
			configYAML:  resolveTestYAML,
			wantName:    "aws:prod",
			wantProfile: "prod-sso",
		},
		{
			name:       "flag with full name",
			flag:       "gcp:staging",
			configYAML: resolveTestYAML,
			wantName:   "gcp:staging",
		},
		{
			name:       "flag unknown",
			flag:       "nope",
			configYAML: resolveTestYAML,
			wantErr:    `context "nope" not found`,
		},
		{
			name:        "no flag with current set",
			flag:        "",
			configYAML:  resolveTestYAML,
			wantName:    "aws:prod",
			wantProfile: "prod-sso",
		},
		{
			name:       "no flag and none set",
			flag:       "",
			configYAML: resolveTestNoCurrentYAML,
			wantErr:    "no context set. Use 'cml use <context>' to set one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateHome(t)
			writeTestFile(t, GetCMLConfigPath(), tt.configYAML, 0600)

			ctxConfig, ctxName, err := ResolveContext(tt.flag)

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
