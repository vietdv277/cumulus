package cmd

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vietdv277/cumulus/internal/config"
)

// captureStdout returns what fn writes to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

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

	var runErr error
	out := captureStdout(t, func() { runErr = runUse(useCmd, []string{"stg"}) })
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if want := "Switched to context: gcp:staging (alias stg)"; !strings.Contains(out, want) {
		t.Fatalf("output = %q, want it to contain %q", out, want)
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "gcp:staging" {
		t.Fatalf("current context = %q, want gcp:staging", cfg.CurrentContext)
	}
}

func TestRunUseFullNamePrintsNoAlias(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	var runErr error
	out := captureStdout(t, func() { runErr = runUse(useCmd, []string{"gcp:staging"}) })
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if !strings.Contains(out, "Switched to context: gcp:staging\n") || strings.Contains(out, "alias") {
		t.Fatalf("output = %q, want a plain switch message", out)
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
