// Aliases: short names that resolve to context names, stored in
// CMLConfig.Aliases. Pure helpers over an already-loaded config; callers
// persist changes through SaveCMLConfig.
package config

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAliasNotFound is returned when removing an alias that does not exist.
var ErrAliasNotFound = errors.New("alias not found")

// ValidateAliasName rejects empty alias names and names containing ':',
// which would be confusable with <provider>:<name> context names.
func ValidateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name must not be empty")
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("alias name %q must not contain ':'", name)
	}
	return nil
}

// AddAlias validates and adds name → target to cfg.Aliases. The target must
// be an existing context (aliases do not chain) and the name must not shadow
// a context; overwriting an existing alias requires force.
func AddAlias(cfg *CMLConfig, name, target string, force bool) error {
	if err := ValidateAliasName(name); err != nil {
		return err
	}
	if _, ok := cfg.Contexts[name]; ok {
		return fmt.Errorf("alias %q would shadow an existing context name", name)
	}
	if _, ok := cfg.Contexts[target]; !ok {
		return fmt.Errorf("context %q not found — alias targets must be real context names", target)
	}
	if _, exists := cfg.Aliases[name]; exists && !force {
		return fmt.Errorf("alias %q already exists — pass --force to overwrite", name)
	}
	if cfg.Aliases == nil {
		cfg.Aliases = make(map[string]string)
	}
	cfg.Aliases[name] = target
	return nil
}

// RemoveAlias deletes name from cfg.Aliases.
func RemoveAlias(cfg *CMLConfig, name string) error {
	if _, ok := cfg.Aliases[name]; !ok {
		return fmt.Errorf("%w: %q", ErrAliasNotFound, name)
	}
	delete(cfg.Aliases, name)
	return nil
}

// DanglingAliases returns the aliases whose target context no longer exists,
// keyed by alias name.
func DanglingAliases(cfg *CMLConfig) map[string]string {
	dangling := make(map[string]string)
	for name, target := range cfg.Aliases {
		if _, ok := cfg.Contexts[target]; !ok {
			dangling[name] = target
		}
	}
	return dangling
}
