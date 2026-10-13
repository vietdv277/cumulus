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

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

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

func TestRunUseNotFoundSuggestsOnce(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	var runErr error
	stderr := captureStderr(t, func() { runErr = runUse(useCmd, []string{"aws:prd"}) })
	if !errors.Is(runErr, config.ErrContextNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, config.ErrContextNotFound)", runErr)
	}

	// The suggestion lives in the stderr block; the returned error omits it
	// so it is not printed twice.
	if want := `context not found: "aws:prd"`; runErr.Error() != want {
		t.Fatalf("err = %q, want %q", runErr.Error(), want)
	}
	if want := `Context "aws:prd" not found — did you mean "aws:prod"?`; !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
	}
	if n := strings.Count(stderr, "did you mean"); n != 1 {
		t.Fatalf("suggestion printed %d times in stderr, want 1:\n%s", n, stderr)
	}
}

func TestRunUseNotFoundNoSuggestion(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	var runErr error
	stderr := captureStderr(t, func() { runErr = runUse(useCmd, []string{"zzzzzz"}) })
	if !errors.Is(runErr, config.ErrContextNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, config.ErrContextNotFound)", runErr)
	}
	if want := `context not found: "zzzzzz"`; runErr.Error() != want {
		t.Fatalf("err = %q, want %q", runErr.Error(), want)
	}
	if want := "Context \"zzzzzz\" not found.\n"; !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
	}
	if strings.Contains(stderr, "did you mean") {
		t.Fatalf("stderr = %q, want no suggestion", stderr)
	}
}

func TestRunUseUpdateNotFoundSuggests(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	err := runUseUpdate(useUpdateCmd, []string{"aws:prd"})
	if err == nil {
		t.Fatal("expected error for an unknown context")
	}
	want := `context "aws:prd" not found — did you mean "aws:prod"? — use 'cml use add' to create it`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}

	err = runUseUpdate(useUpdateCmd, []string{"zzzzzz"})
	if err == nil {
		t.Fatal("expected error for an unknown context")
	}
	want = `context "zzzzzz" not found — use 'cml use add' to create it`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRunUseDanglingAliasKeepsResolvedName(t *testing.T) {
	// alias prod → aws:prod, but aws:prod is gone: the returned error must
	// keep naming the resolved context, exactly like today.
	setTestConfig(t, `current_context: gcp:staging
contexts:
  gcp:staging:
    provider: gcp
    project: my-project
aliases:
  prod: aws:prod
`)

	var runErr error
	stderr := captureStderr(t, func() { runErr = runUse(useCmd, []string{"prod"}) })
	if !errors.Is(runErr, config.ErrContextNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, config.ErrContextNotFound)", runErr)
	}
	if want := `context not found: "aws:prod"`; runErr.Error() != want {
		t.Fatalf("err = %q, want %q", runErr.Error(), want)
	}
	if want := "Context \"prod\" not found.\n"; !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
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
