// Package config loads and saves the context-aware CLI configuration at
// ~/.config/cml/config.yaml (XDG-compliant). A context encodes a cloud
// provider plus credentials reference (AWS profile or GCP project) and region.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vietdv277/cumulus/internal/suggest"
)

// ErrContextNotFound is returned when a named context does not exist.
var ErrContextNotFound = errors.New("context not found")

// Context represents a cloud context configuration
type Context struct {
	Provider string `yaml:"provider"`          // "aws" or "gcp"
	Profile  string `yaml:"profile,omitempty"` // AWS profile name
	Project  string `yaml:"project,omitempty"` // GCP project ID
	Region   string `yaml:"region,omitempty"`  // Region or zone
	// Bastion host settings (AWS: EC2 instance ID; GCP: VM name)
	Bastion        string `yaml:"bastion,omitempty"`
	BastionPort    int    `yaml:"bastion_port,omitempty"`    // remote port on bastion (AWS k8s connect; default 8888)
	BastionProject string `yaml:"bastion_project,omitempty"` // GCP only
	BastionZone    string `yaml:"bastion_zone,omitempty"`    // GCP only
	BastionIAP     bool   `yaml:"bastion_iap,omitempty"`     // GCP only: --tunnel-through-iap
}

// TunnelConfig represents a saved tunnel configuration
type TunnelConfig struct {
	Context    string `yaml:"context"`
	Bastion    string `yaml:"bastion,omitempty"`
	Target     string `yaml:"target,omitempty"`
	RemoteHost string `yaml:"remote_host,omitempty"`
	RemotePort int    `yaml:"remote_port"`
	LocalPort  int    `yaml:"local_port"`
}

// Defaults represents default settings
type Defaults struct {
	Output         string `yaml:"output,omitempty"`          // table, json, yaml
	Interactive    bool   `yaml:"interactive,omitempty"`     // Default interactive mode
	RegionFallback string `yaml:"region_fallback,omitempty"` // Fallback region
}

// CMLConfig represents the main configuration file (~/.config/cml/config.yaml)
type CMLConfig struct {
	CurrentContext string                   `yaml:"current_context,omitempty"`
	Contexts       map[string]*Context      `yaml:"contexts,omitempty"`
	Aliases        map[string]string        `yaml:"aliases,omitempty"`
	Tunnels        map[string]*TunnelConfig `yaml:"tunnels,omitempty"`
	Defaults       *Defaults                `yaml:"defaults,omitempty"`
}

// GetCMLConfigDir returns ~/.config/cml, respecting $XDG_CONFIG_HOME if set.
// This uses the XDG convention consistently across all platforms instead of
// os.UserConfigDir(), which returns ~/Library/Application Support on macOS.
func GetCMLConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "cml")
	}
	return filepath.Join(home, ".config", "cml")
}

// GetCMLConfigPath returns ~/.config/cml/config.yaml (XDG-compliant)
func GetCMLConfigPath() string {
	return filepath.Join(GetCMLConfigDir(), "config.yaml")
}

// LoadCMLConfig loads the configuration from ~/.config/cml/config.yaml
func LoadCMLConfig() (*CMLConfig, error) {
	configPath := GetCMLConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return &CMLConfig{
				Contexts: make(map[string]*Context),
				Aliases:  make(map[string]string),
				Tunnels:  make(map[string]*TunnelConfig),
				Defaults: &Defaults{
					Output:      "table",
					Interactive: false,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg CMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Initialize maps if nil
	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]*Context)
	}
	if cfg.Aliases == nil {
		cfg.Aliases = make(map[string]string)
	}
	if cfg.Tunnels == nil {
		cfg.Tunnels = make(map[string]*TunnelConfig)
	}
	if cfg.Defaults == nil {
		cfg.Defaults = &Defaults{Output: "table"}
	}

	return &cfg, nil
}

// writeFileAtomic writes data to path atomically with restrictive
// permissions: the containing directory is created 0700 and the file is
// written 0600 via a temp file and rename, which also tightens pre-existing
// 0644 files. Used by every config write site.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	// MkdirAll keeps an existing directory's mode, so tighten one left group/world accessible.
	if info, statErr := os.Stat(dir); statErr == nil && info.Mode().Perm()&0077 != 0 {
		if err = os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("chmod config directory: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Chmod(0600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

// SaveCMLConfig saves the configuration to ~/.config/cml/config.yaml
func SaveCMLConfig(cfg *CMLConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFileAtomic(GetCMLConfigPath(), data); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCurrentContext returns the current active context
func GetCurrentContext() (*Context, string, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, "", err
	}

	if cfg.CurrentContext == "" {
		return nil, "", nil
	}

	ctx, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok {
		return nil, "", fmt.Errorf("context %q not found%s", cfg.CurrentContext, ContextSuggestion(cfg, cfg.CurrentContext))
	}

	return ctx, cfg.CurrentContext, nil
}

// SetCurrentContext sets the current active context, resolving aliases first.
func SetCurrentContext(name string) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}

	name = ResolveAliasFrom(cfg, name)

	// Validate context exists
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("%w: %q%s", ErrContextNotFound, name, ContextSuggestion(cfg, name))
	}

	cfg.CurrentContext = name
	return SaveCMLConfig(cfg)
}

// AddContext adds or updates a context
func AddContext(name string, ctx *Context) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}

	cfg.Contexts[name] = ctx
	return SaveCMLConfig(cfg)
}

// DeleteContext removes a context
func DeleteContext(name string) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}

	delete(cfg.Contexts, name)

	// Clear current context if it was the deleted one
	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
	}

	return SaveCMLConfig(cfg)
}

// ListContexts returns all configured contexts
func ListContexts() (map[string]*Context, string, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, "", err
	}

	return cfg.Contexts, cfg.CurrentContext, nil
}

// ParseContextName parses a context name like "aws:prod" into provider and name
func ParseContextName(name string) (provider, contextName string) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", name
}

// ResolveAlias resolves an alias to its target
func ResolveAlias(alias string) (string, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return "", err
	}

	return ResolveAliasFrom(cfg, alias), nil
}

// ResolveAliasFrom resolves an alias against an already-loaded config,
// returning the original name when it is not an alias.
func ResolveAliasFrom(cfg *CMLConfig, alias string) string {
	if target, ok := cfg.Aliases[alias]; ok {
		return target
	}

	return alias
}

// suggestSuffix formats the "did you mean" clause with its leading em dash,
// or "" when no candidate is close enough to suggest.
func suggestSuffix(name string, candidates []string) string {
	if hint := suggest.DidYouMean(suggest.Similar(name, candidates, 3)); hint != "" {
		return " — " + hint
	}
	return ""
}

// ContextSuggestion returns the " — did you mean ...?" suffix for a context or
// alias name that matched nothing, or "" when no configured name is close.
// Exported because cmd builds some not-found errors itself.
func ContextSuggestion(cfg *CMLConfig, name string) string {
	return suggestSuffix(name, contextCandidates(cfg))
}

// contextNames returns the configured context names.
func contextNames(cfg *CMLConfig) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	return names
}

// contextCandidates returns every name a context lookup could have meant:
// context names plus alias names.
func contextCandidates(cfg *CMLConfig) []string {
	names := contextNames(cfg)
	for name := range cfg.Aliases {
		names = append(names, name)
	}
	return names
}

// GetTunnelConfig returns a saved tunnel configuration
func GetTunnelConfig(name string) (*TunnelConfig, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, err
	}

	tunnel, ok := cfg.Tunnels[name]
	if !ok {
		return nil, fmt.Errorf("tunnel %q not found%s", name, suggestSuffix(name, tunnelCandidates(cfg)))
	}

	return tunnel, nil
}

// MigrateFromMacOSConfig migrates from ~/Library/Application Support/cml/config.yaml
// (the macOS os.UserConfigDir() path) to ~/.config/cml/config.yaml.
// It is a no-op if the old file doesn't exist or the new file already exists.
func MigrateFromMacOSConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	oldPath := filepath.Join(home, "Library", "Application Support", "cml", "config.yaml")

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return nil
	}

	newPath := GetCMLConfigPath()

	// New file already exists; leave both alone
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read macOS config: %w", err)
	}
	if err := writeFileAtomic(newPath, data); err != nil {
		return fmt.Errorf("write migrated config: %w", err)
	}

	if err := os.Remove(oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove legacy macOS config %s: %v\n", oldPath, err)
	}

	return nil
}

// MigrateFromDotFileConfig migrates from ~/.cml.yaml to ~/.config/cml/config.yaml.
// It is a no-op if the old file doesn't exist or the new file already exists.
func MigrateFromDotFileConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	oldPath := filepath.Join(home, ".cml.yaml")

	// No old file to migrate
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return nil
	}

	newPath := GetCMLConfigPath()

	// New file already exists; leave both alone
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}

	// Raw byte copy to preserve comments/formatting
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read legacy config: %w", err)
	}
	if err := writeFileAtomic(newPath, data); err != nil {
		return fmt.Errorf("write migrated config: %w", err)
	}

	// Remove old file; warn but don't fail if removal fails
	if err := os.Remove(oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove legacy config %s: %v\n", oldPath, err)
	}

	return nil
}

// MigrateFromOldConfig migrates from the old ~/.cml/config.yaml format to the new
// ~/.config/cml/config.yaml format. SaveCMLConfig writes to the XDG path automatically.
func MigrateFromOldConfig() error {
	oldCfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("read legacy config: %w", err)
	}

	// Check if old config has any data
	if oldCfg.AWSProfile == "" && oldCfg.AWSRegion == "" {
		return nil
	}

	// Load or create new config; only a missing file means "start empty",
	// any other load error must abort the migration rather than wipe it.
	newCfg, err := LoadCMLConfig()
	if err != nil {
		return fmt.Errorf("load new config: %w", err)
	}

	// Create a context from the old config; never overwrite an existing
	// context, only fill the fields it leaves empty.
	if oldCfg.AWSProfile != "" {
		contextName := "aws:" + oldCfg.AWSProfile
		if existing, ok := newCfg.Contexts[contextName]; ok {
			if existing.Profile == "" {
				existing.Profile = oldCfg.AWSProfile
			}
			if existing.Region == "" {
				existing.Region = oldCfg.AWSRegion
			}
		} else {
			newCfg.Contexts[contextName] = &Context{
				Provider: "aws",
				Profile:  oldCfg.AWSProfile,
				Region:   oldCfg.AWSRegion,
			}
		}

		// Set as current if no current context
		if newCfg.CurrentContext == "" {
			newCfg.CurrentContext = contextName
		}
	}

	if err := SaveCMLConfig(newCfg); err != nil {
		return err
	}

	// Remove old file so this migration doesn't re-run on every invocation
	oldPath := GetConfigPath()
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: could not remove legacy config %s: %v\n", oldPath, err)
	}

	return nil
}
