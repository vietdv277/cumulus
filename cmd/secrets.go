package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var secretsCmd = &cobra.Command{
	Use:     "secrets",
	Aliases: []string{"secret"},
	Short:   "Manage secrets and parameters",
	Long: `Manage secrets across cloud providers.

For AWS, this manages both SSM Parameter Store and Secrets Manager.
For GCP, this manages Secret Manager.

Commands operate within the current context. Use 'cml use <context>' to switch.

Examples:
  cml secrets list                     # List all secrets
  cml secrets list /app/               # List secrets with prefix
  cml secrets get /app/db-password     # Get secret value
  cml secrets set /app/new-param val   # Create/update secret
  cml secrets delete /app/old-param    # Delete secret`,
}

var secretsListCmd = &cobra.Command{
	Use:     "list [prefix]",
	Aliases: []string{"ls"},
	Short:   "List secrets",
	Long: `List secrets in the current context.

For AWS:
- Secrets starting with / are from SSM Parameter Store
- Other secrets are from Secrets Manager

Examples:
  cml secrets list                     # List all secrets
  cml secrets list /app/               # List SSM parameters with prefix
  cml secrets list --ssm-only          # List only SSM parameters
  cml secrets list --sm-only           # List only Secrets Manager secrets`,
	RunE: runSecretsList,
}

var secretsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get secret value",
	Long: `Get the value of a secret.

For AWS:
- Secrets starting with / are retrieved from SSM Parameter Store
- Other secrets are retrieved from Secrets Manager

Examples:
  cml secrets get /app/db-password
  cml secrets get my-api-key`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretsGet,
}

var secretsSetCmd = &cobra.Command{
	Use:   "set <name> <value>",
	Short: "Create or update a secret",
	Long: `Create or update a secret.

For AWS:
- Names starting with / are stored in SSM Parameter Store
- Other names are stored in Secrets Manager

Examples:
  cml secrets set /app/db-password "mysecret"
  cml secrets set my-api-key "abc123"`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretsSet,
}

var secretsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Long: `Delete a secret.

Examples:
  cml secrets delete /app/old-param
  cml secrets delete my-old-secret`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretsDelete,
}

var (
	secretsContextFlag string
	secretsSSMOnly     bool
	secretsSMOnly      bool
)

func init() {
	rootCmd.AddCommand(secretsCmd)
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsSetCmd)
	secretsCmd.AddCommand(secretsDeleteCmd)

	// List flags
	secretsListCmd.Flags().BoolVar(&secretsSSMOnly, "ssm-only", false, "List only SSM Parameter Store secrets")
	secretsListCmd.Flags().BoolVar(&secretsSMOnly, "sm-only", false, "List only Secrets Manager secrets")

	// Global context override
	secretsCmd.PersistentFlags().StringVarP(&secretsContextFlag, "context", "c", "", "Use specific context")
}

// getSecretsProvider returns the secrets provider for the current or specified context
func getSecretsProvider(ctx context.Context) (provider.SecretsProvider, error) {
	var ctxConfig *config.Context
	var ctxName string
	var err error

	if secretsContextFlag != "" {
		cfg, loadErr := config.LoadCMLConfig()
		if loadErr != nil {
			return nil, loadErr
		}
		ctxConfig = cfg.Contexts[secretsContextFlag]
		if ctxConfig == nil {
			return nil, fmt.Errorf("context %q not found", secretsContextFlag)
		}
		ctxName = secretsContextFlag
	} else {
		ctxConfig, ctxName, err = config.GetCurrentContext()
		if err != nil {
			return nil, err
		}
		if ctxConfig == nil {
			return nil, fmt.Errorf("no context set. Use 'cml use <context>' to set one")
		}
	}

	switch ctxConfig.Provider {
	case "aws":
		client, err := aws.NewClient(ctx,
			aws.WithProfile(ctxConfig.Profile),
			aws.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create AWS client: %w", err)
		}

		// Create SSM and Secrets Manager clients
		ssmClient := ssm.NewFromConfig(client.Config())
		smClient := secretsmanager.NewFromConfig(client.Config())

		return aws.NewSecretsProvider(client, ssmClient, smClient, ctxConfig.Profile, ctxConfig.Region), nil

	case "gcp":
		return nil, fmt.Errorf("GCP Secrets provider not yet implemented")

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

func runSecretsList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	secretsProvider, err := getSecretsProvider(ctx)
	if err != nil {
		return err
	}

	// Build filter
	filter := &provider.SecretFilter{}
	if len(args) > 0 {
		filter.Prefix = args[0]
	}

	// List secrets
	secrets, err := secretsProvider.List(ctx, filter)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	// Print table
	printSecretsTable(secrets)

	return nil
}

func runSecretsGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	secretsProvider, err := getSecretsProvider(ctx)
	if err != nil {
		return err
	}

	secretValue, err := secretsProvider.Get(ctx, args[0])
	if err != nil {
		return err
	}

	// Print secret details
	printSecretDetails(secretValue)

	return nil
}

func runSecretsSet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	secretsProvider, err := getSecretsProvider(ctx)
	if err != nil {
		return err
	}

	name := args[0]
	value := args[1]

	if err := secretsProvider.Set(ctx, name, value); err != nil {
		return err
	}

	fmt.Printf("Secret set: %s\n", name)
	return nil
}

func runSecretsDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	secretsProvider, err := getSecretsProvider(ctx)
	if err != nil {
		return err
	}

	name := args[0]

	if err := secretsProvider.Delete(ctx, name); err != nil {
		return err
	}

	fmt.Printf("Secret deleted: %s\n", name)
	return nil
}

// printSecretsTable prints secrets in a table format
func printSecretsTable(secrets []types.Secret) {
	headers := []string{"Name", "ARN/Type", "Updated"}
	widths := []int{45, 45, 18}

	var sb strings.Builder

	// Top border
	sb.WriteString(ui.BorderStyle.Render(ui.TopLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.TopT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	for i, h := range headers {
		cell := " " + padRightSecrets(h, widths[i]) + " "
		sb.WriteString(ui.HeaderStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(ui.BorderStyle.Render(ui.LeftT))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.Cross))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.RightT))
	sb.WriteString("\n")

	// Data rows
	for _, secret := range secrets {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Name
		cell := " " + padRightSecrets(secret.Name, widths[0]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// ARN/Type
		arnOrType := secret.ARN
		if arnOrType == secret.Name {
			arnOrType = "SSM Parameter"
		} else if strings.Contains(arnOrType, "secretsmanager") {
			arnOrType = "Secrets Manager"
		}
		cell = " " + padRightSecrets(arnOrType, widths[1]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Updated
		updated := ""
		if !secret.UpdatedAt.IsZero() {
			updated = secret.UpdatedAt.Format("2006-01-02 15:04")
		}
		cell = " " + padRightSecrets(updated, widths[2]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(ui.BorderStyle.Render(ui.BottomLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.BottomT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d secrets\n", len(secrets))
}

func printSecretDetails(sv *types.SecretValue) {
	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("Secret Details"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  Name:     %s\n", ui.NameStyle.Render(sv.Name))
	if sv.ARN != sv.Name {
		fmt.Printf("  ARN:      %s\n", ui.MutedStyle.Render(sv.ARN))
	}
	if sv.Version != "" {
		fmt.Printf("  Version:  %s\n", sv.Version)
	}
	if !sv.UpdatedAt.IsZero() {
		fmt.Printf("  Updated:  %s\n", sv.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	if !sv.CreatedAt.IsZero() && sv.CreatedAt != sv.UpdatedAt {
		fmt.Printf("  Created:  %s\n", sv.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  Provider: %s\n", formatProviderName(sv.Provider))
	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("Value"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Println(sv.Value)
}

func padRightSecrets(s string, width int) string {
	if len(s) >= width {
		if width > 3 {
			return s[:width-3] + "..."
		}
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}
