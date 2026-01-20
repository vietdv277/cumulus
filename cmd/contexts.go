package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var contextsCmd = &cobra.Command{
	Use:     "contexts",
	Aliases: []string{"ctx"},
	Short:   "List all configured contexts",
	Long: `List all configured cloud contexts.

The current active context is marked with an asterisk (*).

Examples:
  cml contexts
  cml ctx`,
	RunE: runContexts,
}

func init() {
	rootCmd.AddCommand(contextsCmd)
}

func runContexts(cmd *cobra.Command, args []string) error {
	contexts, current, err := config.ListContexts()
	if err != nil {
		return fmt.Errorf("failed to list contexts: %w", err)
	}

	if len(contexts) == 0 {
		fmt.Println("No contexts configured.")
		fmt.Println()
		fmt.Println("Add a context with:")
		fmt.Println("  cml use add aws:prod --profile <profile> --region <region>")
		fmt.Println("  cml use add gcp:prod --project <project-id> --region <region>")
		return nil
	}

	// Sort context names
	names := make([]string, 0, len(contexts))
	for name := range contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	// Print header
	fmt.Println()
	fmt.Printf("  %-20s  %-8s  %-20s  %-20s\n",
		ui.HeaderStyle.Render("CONTEXT"),
		ui.HeaderStyle.Render("PROVIDER"),
		ui.HeaderStyle.Render("PROFILE/PROJECT"),
		ui.HeaderStyle.Render("REGION"))
	fmt.Println(ui.MutedStyle.Render("  " + strings.Repeat("─", 75)))

	// Print contexts
	for _, name := range names {
		ctx := contexts[name]

		// Marker for current context
		marker := "  "
		if name == current {
			marker = "* "
		}

		// Format provider
		providerStr := formatProviderShort(ctx.Provider)

		// Get profile/project
		credential := ctx.Profile
		if ctx.Project != "" {
			credential = ctx.Project
		}

		// Format region
		region := ctx.Region
		if region == "" {
			region = ui.MutedStyle.Render("-")
		}

		// Format name with marker
		nameStr := name
		if name == current {
			nameStr = ui.RunningStyle.Render(name)
		}

		fmt.Printf("%s%-20s  %-8s  %-20s  %-20s\n",
			marker,
			nameStr,
			providerStr,
			credential,
			region)
	}

	fmt.Println()
	fmt.Printf("  %d contexts configured", len(contexts))
	if current != "" {
		fmt.Printf(", current: %s", ui.RunningStyle.Render(current))
	}
	fmt.Println()

	return nil
}

func formatProviderShort(provider string) string {
	switch provider {
	case "aws":
		return ui.AWSStyle.Render("AWS")
	case "gcp":
		return ui.GCPStyle.Render("GCP")
	default:
		return provider
	}
}
