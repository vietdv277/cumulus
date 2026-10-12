package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vietdv277/cumulus/internal/config"
)

const aliasTestYAML = `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
  gcp:staging:
    provider: gcp
    project: my-project
    region: asia-southeast1
  legacy:
    provider: aws
    profile: legacy
aliases:
  stg: gcp:staging
`

const aliasDanglingYAML = `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
aliases:
  old: aws:prd
`

// setAliasForce resets the --force flag var so one test cannot leak into another.
func setAliasForce(t *testing.T, force bool) {
	t.Helper()
	prev := aliasAddForce
	aliasAddForce = force
	t.Cleanup(func() { aliasAddForce = prev })
}

func TestRunAliasAdd(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		force     bool
		wantErr   string
		wantAlias string // "<name>=<target>" persisted when set
	}{
		{
			name:      "adds an alias",
			args:      []string{"prod", "aws:prod"},
			wantAlias: "prod=aws:prod",
		},
		{
			name:    "rejects a colon in the name",
			args:    []string{"pro:d", "aws:prod"},
			wantErr: "must not contain ':'",
		},
		{
			name:    "rejects shadowing a context name",
			args:    []string{"legacy", "aws:prod"},
			wantErr: "would shadow",
		},
		{
			name:    "rejects a missing target context",
			args:    []string{"x", "aws:nope"},
			wantErr: `context "aws:nope" not found`,
		},
		{
			name:    "rejects an alias as target",
			args:    []string{"x", "stg"},
			wantErr: `context "stg" not found`,
		},
		{
			name:    "rejects overwrite without force",
			args:    []string{"stg", "aws:prod"},
			wantErr: "--force",
		},
		{
			name:      "force overwrites",
			args:      []string{"stg", "aws:prod"},
			force:     true,
			wantAlias: "stg=aws:prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestConfig(t, aliasTestYAML)
			setAliasForce(t, tt.force)

			err := runAliasAdd(aliasAddCmd, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				cfg, cfgErr := config.LoadCMLConfig()
				if cfgErr != nil {
					t.Fatal(cfgErr)
				}
				if _, changed := cfg.Aliases["prod"]; changed {
					t.Fatal("config was modified despite the error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			name, target, _ := strings.Cut(tt.wantAlias, "=")
			cfg, cfgErr := config.LoadCMLConfig()
			if cfgErr != nil {
				t.Fatal(cfgErr)
			}
			if cfg.Aliases[name] != target {
				t.Fatalf("aliases[%s] = %q, want %q (persisted through save)", name, cfg.Aliases[name], target)
			}
		})
	}
}

func TestRunAliasRm(t *testing.T) {
	t.Run("removes an existing alias", func(t *testing.T) {
		setTestConfig(t, aliasTestYAML)

		if err := runAliasRm(aliasRmCmd, []string{"stg"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cfg, err := config.LoadCMLConfig()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := cfg.Aliases["stg"]; ok {
			t.Fatal("alias was not removed")
		}
	})

	t.Run("missing alias fails", func(t *testing.T) {
		setTestConfig(t, aliasTestYAML)

		err := runAliasRm(aliasRmCmd, []string{"nope"})
		if !errors.Is(err, config.ErrAliasNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, config.ErrAliasNotFound)", err)
		}
	})
}

func TestRunAliasListTable(t *testing.T) {
	setTestConfig(t, aliasDanglingYAML)

	out := captureStdout(t, func() { _ = runAliasList(aliasListCmd, nil) })
	for _, want := range []string{"ALIAS", "CONTEXT", "old", "aws:prd (missing)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want it to contain %q", out, want)
		}
	}
}

func TestRunAliasListTableEmpty(t *testing.T) {
	setTestConfig(t, `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
`)

	out := captureStdout(t, func() { _ = runAliasList(aliasListCmd, nil) })
	if !strings.Contains(out, "No aliases configured.") {
		t.Fatalf("output = %q, want an empty-list hint", out)
	}
}

func TestRunAliasListJSON(t *testing.T) {
	setTestConfig(t, aliasTestYAML)
	setOutputFlagState(t, "json")

	out := captureStdout(t, func() { _ = runAliasList(aliasListCmd, nil) })
	var doc []map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not a pure JSON document: %v\ngot: %q", err, out)
	}
	if len(doc) != 1 || doc[0]["name"] != "stg" || doc[0]["context"] != "gcp:staging" || doc[0]["missing"] != false {
		t.Fatalf("unexpected document: %v", doc)
	}
}

func TestRunAliasListJSONEmpty(t *testing.T) {
	setTestConfig(t, `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
`)
	setOutputFlagState(t, "json")

	out := captureStdout(t, func() { _ = runAliasList(aliasListCmd, nil) })
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("output = %q, want []", out)
	}
}
