package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
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

var ctxInteractive bool

func init() {
	rootCmd.AddCommand(contextsCmd)
	contextsCmd.Flags().BoolVarP(&ctxInteractive, "interactive", "i", false, "Interactive selection mode")
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

	if ctxInteractive {
		selected, err := ui.SelectContext(contexts, current)
		if err != nil {
			return nil // cancelled — silent exit
		}
		if selected == current {
			fmt.Printf("Already on context: %s\n", selected)
			return nil
		}
		if err := config.SetCurrentContext(selected); err != nil {
			return err
		}
		fmt.Printf("Switched to context: %s\n", selected)
		return nil
	}

	// Sort context names
	names := make([]string, 0, len(contexts))
	for name := range contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	// Compute column widths from actual content
	w0 := runewidth.StringWidth("CONTEXT")
	w1 := runewidth.StringWidth("PROVIDER")
	w2 := runewidth.StringWidth("PROFILE/PROJECT")
	w3 := runewidth.StringWidth("REGION")
	for _, name := range names {
		ctx := contexts[name]
		cred := ctx.Profile
		if ctx.Project != "" {
			cred = ctx.Project
		}
		region := ctx.Region
		if region == "" {
			region = "-"
		}
		w0 = max(w0, runewidth.StringWidth(name))
		w1 = max(w1, runewidth.StringWidth(strings.ToUpper(ctx.Provider)))
		w2 = max(w2, runewidth.StringWidth(cred))
		w3 = max(w3, runewidth.StringWidth(region))
	}

	// Print header
	fmt.Println()
	fmt.Printf("  %s  %s  %s  %s\n",
		padCtxCol(ui.HeaderStyle.Render("CONTEXT"), "CONTEXT", w0),
		padCtxCol(ui.HeaderStyle.Render("PROVIDER"), "PROVIDER", w1),
		padCtxCol(ui.HeaderStyle.Render("PROFILE/PROJECT"), "PROFILE/PROJECT", w2),
		ui.HeaderStyle.Render("REGION"))
	fmt.Println(ui.MutedStyle.Render("  " + strings.Repeat("─", w0+2+w1+2+w2+2+w3)))

	// Print contexts
	for _, name := range names {
		ctx := contexts[name]

		marker := "  "
		if name == current {
			marker = "* "
		}

		providerPlain := strings.ToUpper(ctx.Provider)
		providerStyled := formatProviderShort(ctx.Provider)

		cred := ctx.Profile
		if ctx.Project != "" {
			cred = ctx.Project
		}

		region := ctx.Region
		regionStyled := region
		if region == "" {
			regionStyled = ui.MutedStyle.Render("-")
		}

		nameStyled := name
		if name == current {
			nameStyled = ui.RunningStyle.Render(name)
		}

		fmt.Printf("%s%s  %s  %s  %s\n",
			marker,
			padCtxCol(nameStyled, name, w0),
			padCtxCol(providerStyled, providerPlain, w1),
			padRightVM(cred, w2),
			regionStyled)
	}

	fmt.Println()
	fmt.Printf("  %d contexts configured", len(contexts))
	if current != "" {
		fmt.Printf(", current: %s", ui.RunningStyle.Render(current))
	}
	fmt.Println()

	return nil
}

// padCtxCol pads a styled string to the given display width using the plain text width.
func padCtxCol(styled, plain string, width int) string {
	pw := runewidth.StringWidth(plain)
	if pw >= width {
		return styled
	}
	return styled + strings.Repeat(" ", width-pw)
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
