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
