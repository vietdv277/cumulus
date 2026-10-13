// Saved tunnel configurations in the `tunnels:` section of the config file.
package config

import (
	"fmt"
	"regexp"
	"sort"
)

// tunnelNameRE restricts saved tunnel names to lowercase letters, digits and
// dashes (no leading dash) so they stay safe as CLI arguments and config keys.
var tunnelNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SaveTunnel stores a named tunnel configuration. Overwriting an existing
// name requires overwrite=true (--force on the command).
func SaveTunnel(name string, tunnel *TunnelConfig, overwrite bool) error {
	if !tunnelNameRE.MatchString(name) {
		return fmt.Errorf("invalid tunnel name %q: use lowercase letters, digits and '-', starting with a letter or digit", name)
	}
	if tunnel == nil {
		return fmt.Errorf("tunnel config is required")
	}
	if tunnel.Context == "" {
		return fmt.Errorf("tunnel %q needs a context", name)
	}

	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}

	if _, exists := cfg.Tunnels[name]; exists && !overwrite {
		return fmt.Errorf("tunnel %q already exists — pass --force to overwrite it", name)
	}
	cfg.Tunnels[name] = tunnel
	return SaveCMLConfig(cfg)
}

// DeleteTunnel removes a saved tunnel configuration.
func DeleteTunnel(name string) error {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Tunnels[name]; !ok {
		return fmt.Errorf("tunnel %q not found%s", name, suggestSuffix(name, tunnelCandidates(cfg)))
	}
	delete(cfg.Tunnels, name)
	return SaveCMLConfig(cfg)
}

// tunnelCandidates returns the saved tunnel names.
func tunnelCandidates(cfg *CMLConfig) []string {
	names := make([]string, 0, len(cfg.Tunnels))
	for name := range cfg.Tunnels {
		names = append(names, name)
	}
	return names
}

// ListTunnels returns every saved tunnel configuration keyed by name, plus
// the names sorted alphabetically for stable display.
func ListTunnels() ([]string, map[string]*TunnelConfig, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(cfg.Tunnels))
	for name := range cfg.Tunnels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, cfg.Tunnels, nil
}
