package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage context aliases",
	Long: `Manage short aliases that resolve to context names.

Aliases work everywhere a context name is accepted: --context, cml use,
use update and use delete.

Examples:
  cml alias list
  cml alias add prod aws:prod
  cml alias rm prod`,
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List aliases",
	Long: `List all aliases as name → context.

Aliases whose target context was deleted are marked (missing).

Examples:
  cml alias list
  cml alias list -o json`,
	RunE: runAliasList,
}

var aliasAddCmd = &cobra.Command{
	Use:   "add <name> <context>",
	Short: "Add an alias for a context",
	Long: `Add an alias that resolves to a full context name.

The target must be an existing context (aliases do not chain), and the alias
name must not contain ':' or shadow an existing context name.

Examples:
  cml alias add prod aws:prod
  cml alias add prod gcp:prod --force   # overwrite an existing alias`,
	Args: cobra.ExactArgs(2),
	RunE: runAliasAdd,
}

var aliasRmCmd = &cobra.Command{
	Use:     "rm <name>",
	Aliases: []string{"remove"},
	Short:   "Remove an alias",
	Long: `Remove an alias.

Examples:
  cml alias rm prod`,
	Args: cobra.ExactArgs(1),
	RunE: runAliasRm,
}

var aliasAddForce bool

func init() {
	rootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasListCmd, aliasAddCmd, aliasRmCmd)

	aliasAddCmd.Flags().BoolVarP(&aliasAddForce, "force", "f", false, "Overwrite an existing alias")
}

// aliasEntry is the stable json/yaml shape of a configured alias.
type aliasEntry struct {
	Name    string `json:"name" yaml:"name"`
	Context string `json:"context" yaml:"context"`
	Missing bool   `json:"missing" yaml:"missing"`
}

func runAliasList(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	names := make([]string, 0, len(cfg.Aliases))
	for name := range cfg.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]aliasEntry, 0, len(names))
	for _, name := range names {
		target := cfg.Aliases[name]
		_, exists := cfg.Contexts[target]
		entries = append(entries, aliasEntry{Name: name, Context: target, Missing: !exists})
	}

	return render(os.Stdout, out, entries, func() { printAliasTable(names, cfg) })
}

func runAliasAdd(cmd *cobra.Command, args []string) error {
	name, target := args[0], args[1]

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.AddAlias(cfg, name, target, aliasAddForce); err != nil {
		return err
	}
	if err := config.SaveCMLConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Alias added: %s → %s\n", name, target)
	return nil
}

func runAliasRm(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.RemoveAlias(cfg, name); err != nil {
		return err
	}
	if err := config.SaveCMLConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Alias removed: %s\n", name)
	return nil
}

// printAliasTable renders the name → context list for table mode, marking
// aliases whose target context no longer exists.
func printAliasTable(names []string, cfg *config.CMLConfig) {
	if len(names) == 0 {
		fmt.Println("No aliases configured.")
		fmt.Println()
		fmt.Println("Add one with:")
		fmt.Println("  cml alias add <name> <context>")
		return
	}

	w := len("ALIAS")
	for _, name := range names {
		if len(name) > w {
			w = len(name)
		}
	}

	fmt.Println()
	fmt.Printf("  %s  %s\n",
		padCtxCol(ui.HeaderStyle.Render("ALIAS"), "ALIAS", w),
		ui.HeaderStyle.Render("CONTEXT"))
	fmt.Println(ui.MutedStyle.Render("  " + strings.Repeat("─", w+2+len("CONTEXT"))))

	for _, name := range names {
		target := cfg.Aliases[name]
		cell := target
		if _, exists := cfg.Contexts[target]; !exists {
			cell = ui.StoppedStyle.Render(target + " (missing)")
		}
		fmt.Printf("  %s  %s\n", padCtxCol(name, name, w), cell)
	}
	fmt.Println()
}
