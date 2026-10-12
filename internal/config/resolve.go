package config

import "fmt"

// ResolveContext returns the context config and its name for a --context
// flag: the flag (alias resolved) wins over the current context. It is the
// shared lookup for context-aware commands, including the cmd/aws package
// which cannot import cmd.
func ResolveContext(contextFlag string) (*Context, string, error) {
	cfg, err := LoadCMLConfig()
	if err != nil {
		return nil, "", err
	}

	if contextFlag != "" {
		name := ResolveAliasFrom(cfg, contextFlag)
		ctxConfig, ok := cfg.Contexts[name]
		if !ok {
			return nil, "", fmt.Errorf("context %q not found — run 'cml contexts' to list configured contexts", name)
		}
		return ctxConfig, name, nil
	}

	if cfg.CurrentContext == "" {
		return nil, "", fmt.Errorf("no context set. Use 'cml use <context>' to set one")
	}
	ctxConfig, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok {
		return nil, "", fmt.Errorf("context %q not found — run 'cml contexts' to list configured contexts", cfg.CurrentContext)
	}
	return ctxConfig, cfg.CurrentContext, nil
}
