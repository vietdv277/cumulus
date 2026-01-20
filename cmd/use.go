package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/config"
)

var useCmd = &cobra.Command{
	Use:   "use <context-name>",
	Short: "Set the active context",
	Long: `Set the active cloud context for subsequent commands.

Context names follow the pattern: <provider>:<name>
Examples: aws:prod, aws:dev, gcp:staging

Once set, all resource commands (vm, db, secrets, etc.) will operate
within this context without needing to specify the provider each time.

Examples:
  cml use aws:prod          # Switch to AWS production context
  cml use gcp:staging       # Switch to GCP staging context
  cml use dev               # Use a context named "dev"`,
	Args: cobra.ExactArgs(1),
	RunE: runUse,
}

var useAddCmd = &cobra.Command{
	Use:   "add <context-name>",
	Short: "Add a new context",
	Long: `Add a new context configuration.

Examples:
  cml use add aws:prod --profile prod-sso --region ap-southeast-1
  cml use add gcp:staging --project mycompany-staging --region asia-southeast1`,
	Args: cobra.ExactArgs(1),
	RunE: runUseAdd,
}

var useDeleteCmd = &cobra.Command{
	Use:   "delete <context-name>",
	Short: "Delete a context",
	Long: `Delete a context configuration.

Examples:
  cml use delete aws:old-env`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"rm", "remove"},
	RunE:    runUseDelete,
}

var (
	// Flags for use add
	useAddProfile string
	useAddProject string
	useAddRegion  string
)

func init() {
	rootCmd.AddCommand(useCmd)
	useCmd.AddCommand(useAddCmd)
	useCmd.AddCommand(useDeleteCmd)

	// Flags for use add
	useAddCmd.Flags().StringVar(&useAddProfile, "profile", "", "AWS profile name")
	useAddCmd.Flags().StringVar(&useAddProject, "project", "", "GCP project ID")
	useAddCmd.Flags().StringVar(&useAddRegion, "region", "", "Region or zone")
}

func runUse(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	// Try to set the context
	if err := config.SetCurrentContext(contextName); err != nil {
		// If context doesn't exist, show helpful message
		contexts, current, listErr := config.ListContexts()
		if listErr != nil {
			return err
		}

		fmt.Printf("Context %q not found.\n\n", contextName)

		if len(contexts) == 0 {
			fmt.Println("No contexts configured. Add one with:")
			fmt.Println("  cml use add aws:prod --profile <profile> --region <region>")
			fmt.Println("  cml use add gcp:prod --project <project-id> --region <region>")
		} else {
			fmt.Println("Available contexts:")
			for name := range contexts {
				marker := "  "
				if name == current {
					marker = "* "
				}
				fmt.Printf("  %s%s\n", marker, name)
			}
		}
		return nil
	}

	// Get the context details to show confirmation
	ctx, _, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	fmt.Printf("Switched to context: %s\n", contextName)
	fmt.Printf("  Provider: %s\n", ctx.Provider)
	if ctx.Profile != "" {
		fmt.Printf("  Profile:  %s\n", ctx.Profile)
	}
	if ctx.Project != "" {
		fmt.Printf("  Project:  %s\n", ctx.Project)
	}
	if ctx.Region != "" {
		fmt.Printf("  Region:   %s\n", ctx.Region)
	}

	return nil
}

func runUseAdd(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	// Parse the context name to determine provider
	provider, _ := config.ParseContextName(contextName)

	// Auto-detect provider from flags if not in name
	if provider == "" {
		if useAddProfile != "" {
			provider = "aws"
		} else if useAddProject != "" {
			provider = "gcp"
		} else {
			return fmt.Errorf("cannot determine provider. Use format 'aws:name' or 'gcp:name', or provide --profile or --project")
		}
	}

	// Validate required fields based on provider
	ctx := &config.Context{
		Provider: provider,
		Region:   useAddRegion,
	}

	switch strings.ToLower(provider) {
	case "aws":
		if useAddProfile == "" {
			return fmt.Errorf("--profile is required for AWS contexts")
		}
		ctx.Profile = useAddProfile
	case "gcp":
		if useAddProject == "" {
			return fmt.Errorf("--project is required for GCP contexts")
		}
		ctx.Project = useAddProject
	default:
		return fmt.Errorf("unknown provider: %s (supported: aws, gcp)", provider)
	}

	// Add the context
	if err := config.AddContext(contextName, ctx); err != nil {
		return fmt.Errorf("failed to add context: %w", err)
	}

	fmt.Printf("Context added: %s\n", contextName)
	fmt.Println("\nTo use this context:")
	fmt.Printf("  cml use %s\n", contextName)

	return nil
}

func runUseDelete(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	if err := config.DeleteContext(contextName); err != nil {
		return fmt.Errorf("failed to delete context: %w", err)
	}

	fmt.Printf("Context deleted: %s\n", contextName)
	return nil
}
