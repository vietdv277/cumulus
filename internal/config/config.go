package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	AWSProfile string `yaml:"aws_profile,omitempty"`
	AWSRegion  string `yaml:"aws_region,omitempty"`
}

// GetConfigDir returns the config directory path (~/.cml)
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cml"
	}
	return filepath.Join(home, ".cml")
}

// GetConfigPath returns the config file path (~/.cml/config.yaml)
func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.yaml")
}

// LoadConfig loads the configuration from ~/.cml/config.yaml
func LoadConfig() (*Config, error) {
	configPath := GetConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves the configuration to ~/.cml/config.yaml
func SaveConfig(cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFileAtomic(GetConfigPath(), data); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetProfile records the active AWS profile in the new XDG config as an
// `aws:<profile>` context and makes it current, so the choice survives
// legacy-config migration. Existing fields of the context are kept.
func SetProfile(profileName string) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	contextName := "aws:" + profileName
	if ctx, ok := cfg.Contexts[contextName]; ok {
		if ctx.Profile == "" {
			ctx.Profile = profileName
		}
	} else {
		cfg.Contexts[contextName] = &Context{
			Provider: "aws",
			Profile:  profileName,
		}
	}
	cfg.CurrentContext = contextName

	return SaveCMLConfig(cfg)
}

// GetSavedProfile returns the active AWS profile: the profile of the current
// context when it is an AWS one, falling back to the legacy ~/.cml/config.yaml
// value. Priority stays --profile flag > config > AWS_PROFILE (callers).
func GetSavedProfile() string {
	if ctx, _, err := GetCurrentContext(); err == nil && ctx != nil && ctx.Provider == "aws" && ctx.Profile != "" {
		return ctx.Profile
	}

	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}
	return cfg.AWSProfile
}
