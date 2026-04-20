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
	"runtime"
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

// LoadContexts returns all contexts plus the current-context name from the
// first existing kubeconfig in KUBECONFIG (colon/semicolon-separated) or,
// if unset, ~/.kube/config. Returns (nil, "", nil) when no file is found.
func LoadContexts() ([]Context, string, error) {
	path, err := resolvePath()
	if err != nil {
		return nil, "", err
	}
	if path == "" {
		return nil, "", nil
	}

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

func resolvePath() (string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		sep := ":"
		if runtime.GOOS == "windows" {
			sep = ";"
		}
		for _, p := range strings.Split(env, sep) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	path := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return path, nil
}
