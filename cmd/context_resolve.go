package cmd

import (
	"fmt"

	"github.com/vietdv277/cumulus/internal/config"
)

// resolveContext returns the context config and its name for context-aware
// commands: the --context flag (alias resolved) wins over the current context.
// The lookup itself lives in config.ResolveContext so the cmd/aws package can
// share it (it cannot import cmd).
func resolveContext(contextFlag string) (*config.Context, string, error) {
	if err := rejectLegacyFlags(); err != nil {
		return nil, "", err
	}
	return config.ResolveContext(contextFlag)
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
