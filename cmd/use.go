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

var useUpdateCmd = &cobra.Command{
	Use:   "update <context-name>",
	Short: "Update fields of an existing context",
	Long: `Update one or more fields of an existing context without replacing it.
Only flags that are explicitly provided are changed; all other fields are preserved.

To remove the bastion from a context, pass --bastion="" explicitly.

Examples:
  cml use update gcp:prod --bastion bastion --bastion-project nexa-infra-np \
      --bastion-zone asia-southeast1-b --bastion-iap
  cml use update aws:prod --region us-west-2
  cml use update gcp:prod --bastion ""    # remove bastion`,
	Args: cobra.ExactArgs(1),
	RunE: runUseUpdate,
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
	useAddProfile     string
	useAddProject     string
	useAddRegion      string
	useAddBastion     string
	useAddBastionProj string
	useAddBastionZone string
	useAddBastionIAP  bool

	// Flags for use update
	useUpdateProfile     string
	useUpdateProject     string
	useUpdateRegion      string
	useUpdateBastion     string
	useUpdateBastionProj string
	useUpdateBastionZone string
	useUpdateBastionIAP  bool
)

func init() {
	rootCmd.AddCommand(useCmd)
	useCmd.AddCommand(useAddCmd)
	useCmd.AddCommand(useUpdateCmd)
	useCmd.AddCommand(useDeleteCmd)

	// Flags for use update
	useUpdateCmd.Flags().StringVar(&useUpdateProfile, "profile", "", "AWS profile name")
	useUpdateCmd.Flags().StringVar(&useUpdateProject, "project", "", "GCP project ID")
	useUpdateCmd.Flags().StringVar(&useUpdateRegion, "region", "", "Region or zone")
	useUpdateCmd.Flags().StringVar(&useUpdateBastion, "bastion", "", "GCP bastion instance name (set to \"\" to remove)")
	useUpdateCmd.Flags().StringVar(&useUpdateBastionProj, "bastion-project", "", "GCP project hosting the bastion")
	useUpdateCmd.Flags().StringVar(&useUpdateBastionZone, "bastion-zone", "", "Zone of the bastion instance")
	useUpdateCmd.Flags().BoolVar(&useUpdateBastionIAP, "bastion-iap", false, "Use --tunnel-through-iap for bastion access")

	// Flags for use add
	useAddCmd.Flags().StringVar(&useAddProfile, "profile", "", "AWS profile name")
	useAddCmd.Flags().StringVar(&useAddProject, "project", "", "GCP project ID")
	useAddCmd.Flags().StringVar(&useAddRegion, "region", "", "Region or zone")
	useAddCmd.Flags().StringVar(&useAddBastion, "bastion", "", "GCP bastion instance name")
	useAddCmd.Flags().StringVar(&useAddBastionProj, "bastion-project", "", "GCP project hosting the bastion (defaults to --project)")
	useAddCmd.Flags().StringVar(&useAddBastionZone, "bastion-zone", "", "Zone of the bastion instance (defaults to --region)")
	useAddCmd.Flags().BoolVar(&useAddBastionIAP, "bastion-iap", false, "Use --tunnel-through-iap for bastion access")
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
	if ctx.Bastion != "" {
		fmt.Printf("  Bastion:  %s\n", ctx.Bastion)
		if ctx.BastionProject != "" {
			fmt.Printf("  Bastion Project: %s\n", ctx.BastionProject)
		}
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
		if useAddBastion != "" {
			ctx.Bastion = useAddBastion
			ctx.BastionProject = useAddBastionProj
			ctx.BastionZone = useAddBastionZone
			ctx.BastionIAP = useAddBastionIAP
		}
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

func runUseUpdate(cmd *cobra.Command, args []string) error {
	contextName := args[0]

	cfg, err := config.LoadCMLConfig()
	if err != nil {
		return err
	}
	ctx, ok := cfg.Contexts[contextName]
	if !ok {
		return fmt.Errorf("context %q not found — use 'cml use add' to create it", contextName)
	}

	changed := func(name string) bool { return cmd.Flags().Changed(name) }

	if changed("profile") {
		ctx.Profile = useUpdateProfile
	}
	if changed("project") {
		ctx.Project = useUpdateProject
	}
	if changed("region") {
		ctx.Region = useUpdateRegion
	}
	if changed("bastion") {
		ctx.Bastion = useUpdateBastion
	}
	if changed("bastion-project") {
		ctx.BastionProject = useUpdateBastionProj
	}
	if changed("bastion-zone") {
		ctx.BastionZone = useUpdateBastionZone
	}
	if changed("bastion-iap") {
		ctx.BastionIAP = useUpdateBastionIAP
	}

	if err := config.SaveCMLConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Context updated: %s\n", contextName)
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
	if ctx.Bastion != "" {
		fmt.Printf("  Bastion:  %s\n", ctx.Bastion)
		if ctx.BastionProject != "" {
			fmt.Printf("  Bastion Project: %s\n", ctx.BastionProject)
		}
	}
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
