package config

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAliasName(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{name: "plain", alias: "prod"},
		{name: "dash", alias: "prod-eu"},
		{name: "empty", alias: "", wantErr: true},
		{name: "colon", alias: "pro:d", wantErr: true},
		{name: "only colon", alias: ":", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAliasName(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAliasName(%q) error = %v, wantErr %v", tt.alias, err, tt.wantErr)
			}
		})
	}
}

// aliasTestCfg returns a config with contexts, one of them without the usual
// <provider>:<name> shape, and one alias.
func aliasTestCfg() *CMLConfig {
	return &CMLConfig{
		Contexts: map[string]*Context{
			"aws:prod": {Provider: "aws", Profile: "prod-sso"},
			"legacy":   {Provider: "aws", Profile: "legacy"},
		},
		Aliases: map[string]string{"old": "aws:prod"},
	}
}

func TestAddAlias(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		target  string
		force   bool
		wantErr string
	}{
		{name: "new alias", alias: "prod", target: "aws:prod"},
		{name: "colon in name", alias: "pro:d", target: "aws:prod",
			wantErr: `must not contain ':'`},
		{name: "shadows a context", alias: "legacy", target: "aws:prod",
			wantErr: "would shadow"},
		{name: "missing target", alias: "prod", target: "aws:nope",
			wantErr: `context "aws:nope" not found`},
		{name: "alias as target", alias: "prod", target: "old",
			wantErr: `context "old" not found`},
		{name: "existing without force", alias: "old", target: "aws:prod",
			wantErr: "--force"},
		{name: "existing with force", alias: "old", target: "aws:prod", force: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := aliasTestCfg()

			err := AddAlias(cfg, tt.alias, tt.target, tt.force)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Aliases[tt.alias] != tt.target {
				t.Fatalf("aliases[%s] = %q, want %q", tt.alias, cfg.Aliases[tt.alias], tt.target)
			}
		})
	}
}

func TestAddAliasCreatesAliasMap(t *testing.T) {
	cfg := &CMLConfig{
		Contexts: map[string]*Context{"aws:prod": {Provider: "aws"}},
	}

	if err := AddAlias(cfg, "prod", "aws:prod", false); err != nil {
		t.Fatalf("AddAlias() error = %v", err)
	}
	if cfg.Aliases["prod"] != "aws:prod" {
		t.Fatalf("aliases = %v, want prod → aws:prod", cfg.Aliases)
	}
}

func TestRemoveAlias(t *testing.T) {
	t.Run("removes an existing alias", func(t *testing.T) {
		cfg := aliasTestCfg()

		if err := RemoveAlias(cfg, "old"); err != nil {
			t.Fatalf("RemoveAlias() error = %v", err)
		}
		if _, ok := cfg.Aliases["old"]; ok {
			t.Fatal("alias was not removed")
		}
	})

	t.Run("missing alias wraps ErrAliasNotFound", func(t *testing.T) {
		cfg := aliasTestCfg()

		err := RemoveAlias(cfg, "nope")
		if !errors.Is(err, ErrAliasNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, ErrAliasNotFound)", err)
		}
	})
}

func TestDanglingAliases(t *testing.T) {
	cfg := &CMLConfig{
		Contexts: map[string]*Context{"aws:prod": {Provider: "aws"}},
		Aliases: map[string]string{
			"ok":        "aws:prod",
			"gone":      "aws:prd",
			"also-gone": "gcp:x",
		},
	}

	got := DanglingAliases(cfg)
	if len(got) != 2 || got["gone"] != "aws:prd" || got["also-gone"] != "gcp:x" {
		t.Fatalf("DanglingAliases() = %v, want only gone and also-gone", got)
	}
}
