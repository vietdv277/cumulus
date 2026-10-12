package cmd

import (
	"errors"
	"testing"

	"github.com/vietdv277/cumulus/internal/config"
)

func TestRunUseNotFoundReturnsError(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	if err := runUse(useCmd, []string{"aws:prd"}); err == nil {
		t.Fatal("expected error for unknown context, got nil")
	} else if !errors.Is(err, config.ErrContextNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, config.ErrContextNotFound)", err)
	}

	// Current context must be unchanged
	cfg, err := config.LoadCMLConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "aws:prod" {
		t.Fatalf("current context = %q, want aws:prod (unchanged)", cfg.CurrentContext)
	}
}

func TestRunUseResolvesAlias(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	if err := runUse(useCmd, []string{"stg"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "gcp:staging" {
		t.Fatalf("current context = %q, want gcp:staging", cfg.CurrentContext)
	}
}

func TestRunUseAddNormalizesProviderCase(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	origProfile, origRegion := useAddProfile, useAddRegion
	t.Cleanup(func() { useAddProfile, useAddRegion = origProfile, origRegion })
	useAddProfile = "prod-sso"
	useAddRegion = "us-east-1"

	if err := runUseAdd(useAddCmd, []string{"AWS:prod"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		t.Fatal(err)
	}
	added, ok := cfg.Contexts["aws:prod"]
	if !ok {
		t.Fatalf("context stored as %v, want key aws:prod", mapKeys(cfg.Contexts))
	}
	if added.Provider != "aws" {
		t.Fatalf("provider = %q, want aws", added.Provider)
	}
}

func TestRunUseDeleteResolvesAlias(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	if err := runUseDelete(useDeleteCmd, []string{"prod"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Contexts["aws:prod"]; ok {
		t.Fatal("aws:prod should be deleted via its alias")
	}
}

func mapKeys(m map[string]*config.Context) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
