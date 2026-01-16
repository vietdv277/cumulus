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
	configDir := GetConfigDir()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := GetConfigPath()
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetProfile updates the AWS profile in the config
func SetProfile(profileName string) error {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = &Config{}
	}

	cfg.AWSProfile = profileName
	return SaveConfig(cfg)
}

// GetSavedProfile returns the saved AWS profile from config
func GetSavedProfile() string {
	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}
	return cfg.AWSProfile
}
