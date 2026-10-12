// Package kubeconfig provides a minimal reader for ~/.kube/config.
//
// It only extracts the fields needed for `cml k8s contexts`: the list of
// contexts and the current-context. Writing is deliberately not supported —
// kubeconfig mutation happens via the cloud CLIs (`aws eks update-kubeconfig`
// and `gcloud container clusters get-credentials`).
package kubeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Context represents a single entry from the `contexts:` list.
type Context struct {
	Name      string
	Cluster   string
	User      string
	Namespace string
}

type rawConfig struct {
	CurrentContext string `yaml:"current-context"`
	Contexts       []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster   string `yaml:"cluster"`
			User      string `yaml:"user"`
			Namespace string `yaml:"namespace"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

// LoadContexts returns all contexts plus the current-context, merged from
// every kubeconfig listed in KUBECONFIG (OS list-separated) or, if unset,
// ~/.kube/config — the way kubectl does it: first file wins for duplicate
// context names and for current-context, missing files are skipped.
// Returns (nil, "", nil) when no file is found.
func LoadContexts() ([]Context, string, error) {
	paths, err := resolvePaths()
	if err != nil {
		return nil, "", err
	}

	var (
		contexts []Context
		current  string
		seen     = make(map[string]bool)
	)
	for _, path := range paths {
		fileContexts, fileCurrent, err := loadFile(path)
		if err != nil {
			return nil, "", err
		}
		for _, c := range fileContexts {
			if seen[c.Name] {
				continue // first file wins for duplicate context names
			}
			seen[c.Name] = true
			contexts = append(contexts, c)
		}
		if current == "" {
			current = fileCurrent
		}
	}
	return contexts, current, nil
}

// loadFile parses a single kubeconfig file.
func loadFile(path string) ([]Context, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read kubeconfig %s: %w", path, err)
	}

	var cfg rawConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("parse kubeconfig %s: %w", path, err)
	}

	contexts := make([]Context, 0, len(cfg.Contexts))
	for _, c := range cfg.Contexts {
		contexts = append(contexts, Context{
			Name:      c.Name,
			Cluster:   c.Context.Cluster,
			User:      c.Context.User,
			Namespace: c.Context.Namespace,
		})
	}
	return contexts, cfg.CurrentContext, nil
}

// resolvePaths returns the candidate kubeconfig files in order: every
// existing file from $KUBECONFIG when set, otherwise ~/.kube/config.
func resolvePaths() ([]string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		var paths []string
		for _, p := range filepath.SplitList(env) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Skip missing files like kubectl does instead of failing.
			if _, err := os.Stat(p); err != nil {
				continue
			}
			paths = append(paths, p)
		}
		return paths, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	path := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return []string{path}, nil
}
