package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Context represents a cloud context configuration
type Context struct {
	Provider string `yaml:"provider"`          // "aws" or "gcp"
	Profile  string `yaml:"profile,omitempty"` // AWS profile name
	Project  string `yaml:"project,omitempty"` // GCP project ID
	Region   string `yaml:"region,omitempty"`  // Region or zone
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

// CMLConfig represents the main configuration file (~/.cml.yaml)
type CMLConfig struct {
	CurrentContext string                  `yaml:"current_context,omitempty"`
	Contexts       map[string]*Context     `yaml:"contexts,omitempty"`
	Aliases        map[string]string       `yaml:"aliases,omitempty"`
	Tunnels        map[string]*TunnelConfig `yaml:"tunnels,omitempty"`
	Defaults       *Defaults               `yaml:"defaults,omitempty"`
}

// GetCMLConfigPath returns the config file path (~/.cml.yaml)
func GetCMLConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cml.yaml"
	}
	return filepath.Join(home, ".cml.yaml")
}

// LoadCMLConfig loads the configuration from ~/.cml.yaml
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

// SaveCMLConfig saves the configuration to ~/.cml.yaml
func SaveCMLConfig(cfg *CMLConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := GetCMLConfigPath()
	if err := os.WriteFile(configPath, data, 0644); err != nil {
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
		return nil, "", fmt.Errorf("context %q not found", cfg.CurrentContext)
	}

	return ctx, cfg.CurrentContext, nil
}

// SetCurrentContext sets the current active context
func SetCurrentContext(name string) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}

	// Validate context exists
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q not found", name)
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

	if target, ok := cfg.Aliases[alias]; ok {
		return target, nil
	}

	return alias, nil // Return original if not an alias
}

// GetTunnelConfig returns a saved tunnel configuration
func GetTunnelConfig(name string) (*TunnelConfig, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, err
	}

	tunnel, ok := cfg.Tunnels[name]
	if !ok {
		return nil, fmt.Errorf("tunnel %q not found", name)
	}

	return tunnel, nil
}

// MigrateFromOldConfig migrates from the old config format to the new one
func MigrateFromOldConfig() error {
	oldCfg, err := LoadConfig()
	if err != nil {
		return nil // No old config to migrate
	}

	// Check if old config has any data
	if oldCfg.AWSProfile == "" && oldCfg.AWSRegion == "" {
		return nil
	}

	// Load or create new config
	newCfg, err := LoadCMLConfig()
	if err != nil {
		newCfg = &CMLConfig{
			Contexts: make(map[string]*Context),
			Aliases:  make(map[string]string),
			Tunnels:  make(map[string]*TunnelConfig),
			Defaults: &Defaults{Output: "table"},
		}
	}

	// Create a context from the old config
	if oldCfg.AWSProfile != "" {
		contextName := "aws:" + oldCfg.AWSProfile
		newCfg.Contexts[contextName] = &Context{
			Provider: "aws",
			Profile:  oldCfg.AWSProfile,
			Region:   oldCfg.AWSRegion,
		}

		// Set as current if no current context
		if newCfg.CurrentContext == "" {
			newCfg.CurrentContext = contextName
		}
	}

	return SaveCMLConfig(newCfg)
}
