package cmd

import (
	"fmt"

	"github.com/vietdv277/cumulus/internal/config"
)

// resolveContext returns the context config and its name for context-aware
// commands: the --context flag (alias resolved) wins over the current context.
// Legacy --profile/--region flags are rejected so they can never be silently
// ignored in favour of the context.
func resolveContext(contextFlag string) (*config.Context, string, error) {
	if err := rejectLegacyFlags(); err != nil {
		return nil, "", err
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		return nil, "", err
	}

	if contextFlag != "" {
		name := config.ResolveAliasFrom(cfg, contextFlag)
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

// rejectLegacyFlags fails fast when -p/--profile or -r/--region were passed
// explicitly. initConfig() fills the package vars from saved config or env even
// when the flag is absent, so only the flag's Changed state counts.
func rejectLegacyFlags() error {
	for _, name := range []string{"profile", "region"} {
		if f := rootCmd.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return fmt.Errorf("--%s is not supported by context-aware commands; use --context <name> or 'cml use <context>'", name)
		}
	}
	return nil
}
