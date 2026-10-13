package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestSaveCMLConfigTightensExistingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	_, xdg := isolateHome(t)
	dir := filepath.Join(xdg, "cml")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := SaveCMLConfig(&CMLConfig{}); err != nil {
		t.Fatalf("SaveCMLConfig() error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("config dir mode = %v, want 0700", got)
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

func TestSetCurrentContextSuggestions(t *testing.T) {
	t.Run("typo suggests contexts and aliases", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := SetCurrentContext("prdo")
		if !errors.Is(err, ErrContextNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, ErrContextNotFound)", err)
		}
		want := `context not found: "prdo" — did you mean "aws:prod" or "prod"?`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("no close match keeps today's text", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := SetCurrentContext("zzzzzz")
		if !errors.Is(err, ErrContextNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, ErrContextNotFound)", err)
		}
		want := `context not found: "zzzzzz"`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})
}

func TestGetCurrentContextSuggestion(t *testing.T) {
	isolateHome(t)
	writeTestFile(t, GetCMLConfigPath(), `current_context: aws:prd
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
`, 0600)

	_, _, err := GetCurrentContext()
	if err == nil {
		t.Fatal("expected error for a stale current context")
	}
	want := `context "aws:prd" not found — did you mean "aws:prod"?`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestDeleteContextSuggestions(t *testing.T) {
	t.Run("unknown name suggests and saves nothing", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := DeleteContext("aws:prd")
		if !errors.Is(err, ErrContextNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, ErrContextNotFound)", err)
		}
		want := `context not found: "aws:prd" — did you mean "aws:prod"?`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
		cfg, cfgErr := LoadCMLConfig()
		if cfgErr != nil {
			t.Fatal(cfgErr)
		}
		if _, ok := cfg.Contexts["aws:prod"]; !ok {
			t.Fatal("config was modified despite the error")
		}
		if cfg.CurrentContext != "aws:prod" {
			t.Fatalf("current context = %q, want aws:prod (unchanged)", cfg.CurrentContext)
		}
	})

	t.Run("unknown name without close match keeps today's text", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := DeleteContext("zzzzzz")
		if !errors.Is(err, ErrContextNotFound) {
			t.Fatalf("err = %v, want errors.Is(err, ErrContextNotFound)", err)
		}
		want := `context not found: "zzzzzz"`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
		cfg, cfgErr := LoadCMLConfig()
		if cfgErr != nil {
			t.Fatal(cfgErr)
		}
		if len(cfg.Contexts) != 2 {
			t.Fatalf("contexts = %d, want 2 (unchanged)", len(cfg.Contexts))
		}
	})

	t.Run("existing context still deletes and clears current", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		if err := DeleteContext("aws:prod"); err != nil {
			t.Fatalf("DeleteContext() error = %v", err)
		}
		cfg, cfgErr := LoadCMLConfig()
		if cfgErr != nil {
			t.Fatal(cfgErr)
		}
		if _, ok := cfg.Contexts["aws:prod"]; ok {
			t.Fatal("aws:prod was not deleted")
		}
		if cfg.CurrentContext != "" {
			t.Fatalf("current context = %q, want empty (the deleted context was current)", cfg.CurrentContext)
		}
	})
}

func TestGetTunnelConfigSuggestions(t *testing.T) {
	t.Run("typo suggests tunnel names", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		_, err := GetTunnelConfig("db-prd")
		if err == nil {
			t.Fatal("expected error for an unknown tunnel")
		}
		want := `tunnel "db-prd" not found — did you mean "db-prod"?`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("no close match keeps today's text", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		_, err := GetTunnelConfig("zzzzzz")
		if err == nil {
			t.Fatal("expected error for an unknown tunnel")
		}
		want := `tunnel "zzzzzz" not found`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})
}
