package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current context and authentication status",
	Long: `Display the current active context and verify authentication status
for the configured cloud provider.

Examples:
  cml status`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Get current context
	ctx, ctxName, err := config.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("failed to get current context: %w", err)
	}

	fmt.Println("Current Status")
	fmt.Println(ui.MutedStyle.Render("─────────────────────────────────"))
	fmt.Println()

	if ctx == nil {
		fmt.Println("Context:  " + ui.MutedStyle.Render("(not set)"))
		fmt.Println()
		fmt.Println("No context configured. Set one with:")
		fmt.Println("  cml use add aws:prod --profile <profile> --region <region>")
		fmt.Println("  cml use aws:prod")
		return nil
	}

	// Display context info
	fmt.Printf("Context:  %s\n", ui.HeaderStyle.Render(ctxName))
	fmt.Printf("Provider: %s\n", formatProvider(ctx.Provider))

	switch ctx.Provider {
	case "aws":
		displayAWSStatus(ctx)
	case "gcp":
		displayGCPStatus(ctx)
	}

	return nil
}

func displayAWSStatus(ctx *config.Context) {
	fmt.Printf("Profile:  %s\n", ui.AWSStyle.Render(ctx.Profile))
	if ctx.Region != "" {
		fmt.Printf("Region:   %s\n", ctx.Region)
	}
	fmt.Println()

	// Try to get caller identity
	fmt.Print("Auth:     ")
	identity, err := aws.GetCallerIdentity(ctx.Profile, ctx.Region)
	if err != nil {
		fmt.Println(ui.StoppedStyle.Render("✗ Not authenticated"))
		fmt.Printf("          %s\n", ui.MutedStyle.Render(err.Error()))
		fmt.Println()
		fmt.Println("To authenticate:")
		fmt.Printf("  aws sso login --profile %s\n", ctx.Profile)
	} else {
		fmt.Println(ui.RunningStyle.Render("✓ Authenticated"))
		fmt.Printf("Account:  %s\n", identity.Account)
		fmt.Printf("User:     %s\n", identity.UserID)
		if identity.Arn != "" {
			fmt.Printf("ARN:      %s\n", ui.MutedStyle.Render(identity.Arn))
		}
	}
}

func displayGCPStatus(ctx *config.Context) {
	fmt.Printf("Project:  %s\n", ui.GCPStyle.Render(ctx.Project))
	if ctx.Region != "" {
		fmt.Printf("Region:   %s\n", ctx.Region)
	}
	fmt.Println()

	// Check for gcloud auth
	fmt.Print("Auth:     ")
	if _, err := os.Stat(os.Getenv("HOME") + "/.config/gcloud/application_default_credentials.json"); err == nil {
		fmt.Println(ui.RunningStyle.Render("✓ Application default credentials found"))
	} else {
		fmt.Println(ui.PendingStyle.Render("? Credentials not verified"))
		fmt.Println()
		fmt.Println("To authenticate:")
		fmt.Println("  gcloud auth application-default login")
	}
}

func formatProvider(provider string) string {
	switch provider {
	case "aws":
		return ui.AWSStyle.Render("AWS")
	case "gcp":
		return ui.GCPStyle.Render("GCP")
	default:
		return provider
	}
}
