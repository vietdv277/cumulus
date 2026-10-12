package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateHome points HOME and XDG_CONFIG_HOME at temp dirs so tests never
// touch the real ~/.config, ~/.cml or ~/.cml.yaml.
func isolateHome(t *testing.T) (home, xdg string) {
	t.Helper()
	home = t.TempDir()
	xdg = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	return home, xdg
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestMigrateFromOldConfigKeepsBrokenNewConfig(t *testing.T) {
	home, xdg := isolateHome(t)

	newPath := filepath.Join(xdg, "cml", "config.yaml")
	broken := "contexts: [oops\n"
	writeTestFile(t, newPath, broken, 0600)

	legacyPath := filepath.Join(home, ".cml", "config.yaml")
	writeTestFile(t, legacyPath, "aws_profile: prod\naws_region: us-east-1\n", 0600)

	if err := MigrateFromOldConfig(); err == nil {
		t.Fatal("expected migration to fail on unparseable new config, got nil")
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new config: %v", err)
	}
	if string(data) != broken {
		t.Errorf("new config was overwritten:\n got %q\nwant %q", data, broken)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("legacy config was removed: %v", err)
	}
}

func TestMigrateFromOldConfigKeepsExistingContext(t *testing.T) {
	_, xdg := isolateHome(t)

	newPath := filepath.Join(xdg, "cml", "config.yaml")
	writeTestFile(t, newPath, `
current_context: gcp:staging
contexts:
  aws:prod:
    provider: aws
    profile: prod
    region: us-west-2
    bastion: i-013xxxxx
  gcp:staging:
    provider: gcp
    project: my-project
`, 0600)

	home := os.Getenv("HOME")
	writeTestFile(t, filepath.Join(home, ".cml", "config.yaml"),
		"aws_profile: prod\naws_region: eu-west-1\n", 0600)

	if err := MigrateFromOldConfig(); err != nil {
		t.Fatalf("MigrateFromOldConfig() error = %v", err)
	}

	cfg, err := LoadCMLConfig()
	if err != nil {
		t.Fatalf("LoadCMLConfig() error = %v", err)
	}
	ctx, ok := cfg.Contexts["aws:prod"]
	if !ok {
		t.Fatal("existing aws:prod context was dropped")
	}
	if ctx.Region != "us-west-2" {
		t.Errorf("region overwritten: got %q, want us-west-2", ctx.Region)
	}
	if ctx.Bastion != "i-013xxxxx" {
		t.Errorf("bastion overwritten: got %q, want i-013xxxxx", ctx.Bastion)
	}
	if cfg.CurrentContext != "gcp:staging" {
		t.Errorf("current context changed: got %q", cfg.CurrentContext)
	}
}

func TestMigrateFromOldConfigCreatesMissingContext(t *testing.T) {
	home, xdg := isolateHome(t)

	newPath := filepath.Join(xdg, "cml", "config.yaml")
	writeTestFile(t, newPath, "", 0600)
	writeTestFile(t, filepath.Join(home, ".cml", "config.yaml"),
		"aws_profile: prod\naws_region: eu-west-1\n", 0600)

	if err := MigrateFromOldConfig(); err != nil {
		t.Fatalf("MigrateFromOldConfig() error = %v", err)
	}

	cfg, err := LoadCMLConfig()
	if err != nil {
		t.Fatalf("LoadCMLConfig() error = %v", err)
	}
	ctx, ok := cfg.Contexts["aws:prod"]
	if !ok {
		t.Fatal("expected aws:prod context to be created")
	}
	if ctx.Region != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1", ctx.Region)
	}
	if cfg.CurrentContext != "aws:prod" {
		t.Errorf("current context = %q, want aws:prod", cfg.CurrentContext)
	}
}

func TestSaveCMLConfigPermissions(t *testing.T) {
	_, xdg := isolateHome(t)
	path := filepath.Join(xdg, "cml", "config.yaml")

	// Fresh save
	if err := SaveCMLConfig(&CMLConfig{}); err != nil {
		t.Fatalf("SaveCMLConfig() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("config file mode = %v, want 0600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Errorf("config dir mode = %v, want 0700", got)
	}

	// Save over a pre-existing world-readable file
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := SaveCMLConfig(&CMLConfig{}); err != nil {
		t.Fatalf("SaveCMLConfig() error = %v", err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("config file mode after overwrite = %v, want 0600", got)
	}
}

func TestSetProfileRoundTrip(t *testing.T) {
	_, _ = isolateHome(t)

	if err := SetProfile("prod"); err != nil {
		t.Fatalf("SetProfile() error = %v", err)
	}

	if got := GetSavedProfile(); got != "prod" {
		t.Errorf("GetSavedProfile() = %q, want prod", got)
	}

	ctx, name, err := GetCurrentContext()
	if err != nil {
		t.Fatalf("GetCurrentContext() error = %v", err)
	}
	if name != "aws:prod" {
		t.Errorf("current context = %q, want aws:prod", name)
	}
	if ctx == nil || ctx.Provider != "aws" || ctx.Profile != "prod" {
		t.Errorf("context = %+v, want provider aws with profile prod", ctx)
	}

	// Reloading from disk must yield the same profile
	cfg, err := LoadCMLConfig()
	if err != nil {
		t.Fatalf("LoadCMLConfig() error = %v", err)
	}
	if cfg.CurrentContext != "aws:prod" {
		t.Errorf("persisted current context = %q, want aws:prod", cfg.CurrentContext)
	}
}

func TestSetProfileKeepsExistingContextFields(t *testing.T) {
	_, xdg := isolateHome(t)

	writeTestFile(t, filepath.Join(xdg, "cml", "config.yaml"), `
current_context: aws:other
contexts:
  aws:prod:
    provider: aws
    profile: prod
    region: us-west-2
    bastion: i-013xxxxx
  aws:other:
    provider: aws
    profile: other
`, 0600)

	if err := SetProfile("prod"); err != nil {
		t.Fatalf("SetProfile() error = %v", err)
	}

	cfg, err := LoadCMLConfig()
	if err != nil {
		t.Fatalf("LoadCMLConfig() error = %v", err)
	}
	ctx, ok := cfg.Contexts["aws:prod"]
	if !ok {
		t.Fatal("aws:prod context missing after profile set")
	}
	if ctx.Region != "us-west-2" || ctx.Bastion != "i-013xxxxx" {
		t.Errorf("existing fields lost: %+v", ctx)
	}
	if cfg.CurrentContext != "aws:prod" {
		t.Errorf("current context = %q, want aws:prod", cfg.CurrentContext)
	}
}

func TestGetSavedProfileFallsBackToLegacyConfig(t *testing.T) {
	home, _ := isolateHome(t)

	writeTestFile(t, filepath.Join(home, ".cml", "config.yaml"),
		"aws_profile: legacy-profile\n", 0600)

	if got := GetSavedProfile(); got != "legacy-profile" {
		t.Errorf("GetSavedProfile() = %q, want legacy-profile", got)
	}
}
