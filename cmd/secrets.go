package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/aws"
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
- Prefix ssm: or sm: forces the backend (e.g. ssm:db-pass, sm:/prod/api-key)

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
- Names starting with / are retrieved from SSM Parameter Store
- Other names are retrieved from Secrets Manager, falling back to SSM
  when not found
- Prefix ssm: or sm: forces the backend (e.g. sm:/prod/api-key)

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
- Other names are stored in Secrets Manager, unless an SSM parameter
  of the same name already exists
- Prefix ssm: or sm: forces the backend (e.g. ssm:db-pass)

Examples:
  cml secrets set /app/db-password "mysecret"
  cml secrets set my-api-key "abc123"`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretsSet,
}

var secretsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Long: `Delete a secret. Asks for confirmation.

Secrets Manager secrets stay recoverable for 7 days after deletion
unless --force is given, which deletes them immediately and
irreversibly. SSM parameters are deleted immediately and cannot be
recovered. Pass --yes to skip the prompt in scripts.

Examples:
  cml secrets delete /app/old-param
  cml secrets delete my-old-secret --force`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretsDelete,
}

var (
	secretsContextFlag string
	secretsSSMOnly     bool
	secretsSMOnly      bool
	secretsDeleteForce bool
	secretsYes         bool
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

	// Delete flags
	secretsDeleteCmd.Flags().BoolVar(&secretsDeleteForce, "force", false, "Delete immediately without a recovery window (Secrets Manager)")
	secretsDeleteCmd.Flags().BoolVarP(&secretsYes, "yes", "y", false, "Skip confirmation prompt")

	// Global context override
	secretsCmd.PersistentFlags().StringVarP(&secretsContextFlag, "context", "c", "", "Use specific context")
}

// getSecretsProvider returns the secrets provider for the current or specified context
func getSecretsProvider(ctx context.Context) (provider.SecretsProvider, error) {
	ctxConfig, ctxName, err := resolveContext(secretsContextFlag)
	if err != nil {
		return nil, err
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
	source, err := secretsListSource(secretsSSMOnly, secretsSMOnly)
	if err != nil {
		return err
	}

	ctx := context.Background()

	secretsProvider, err := getSecretsProvider(ctx)
	if err != nil {
		return err
	}

	// Build filter
	filter := &provider.SecretFilter{Source: source}
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

// secretsListSource maps the --ssm-only/--sm-only flags to a backend selection.
func secretsListSource(ssmOnly, smOnly bool) (provider.SecretSource, error) {
	switch {
	case ssmOnly && smOnly:
		return "", fmt.Errorf("--ssm-only and --sm-only cannot be used together")
	case ssmOnly:
		return provider.SourceSSM, nil
	case smOnly:
		return provider.SourceSM, nil
	default:
		return provider.SourceAll, nil
	}
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
	opts := provider.DeleteSecretOptions{Force: secretsDeleteForce}

	proceed, err := confirmAction(secretsYes, os.Stdin, os.Stdout,
		secretDeletePrompt(name, opts),
		fmt.Sprintf("refusing to delete %s without confirmation; pass --yes", name),
	)
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Println("Delete cancelled")
		return nil
	}

	if err := secretsProvider.Delete(ctx, name, opts); err != nil {
		return err
	}

	// A non-forced Secrets Manager delete only schedules the deletion.
	if backend, _ := aws.ResolveSecretBackend(name); backend == aws.BackendSM && !opts.Force {
		fmt.Printf("Secret scheduled for deletion: %s (recoverable within %d days)\n", name, aws.SecretsManagerRecoveryWindowDays)
		return nil
	}

	fmt.Printf("Secret deleted: %s\n", name)
	return nil
}

// secretDeletePrompt builds the confirmation prompt for secrets delete, stating
// the recovery implications of the backend being deleted from.
func secretDeletePrompt(name string, opts provider.DeleteSecretOptions) string {
	ctxName := contextNameForPrompt(secretsContextFlag)
	backend, stripped := aws.ResolveSecretBackend(name)
	if backend == aws.BackendSSM {
		return fmt.Sprintf("Delete SSM parameter %s in context %s? SSM parameters are deleted permanently and cannot be recovered [y/N]", stripped, ctxName)
	}
	if opts.Force {
		return fmt.Sprintf("Delete secret %s in context %s? This permanently deletes it with no recovery [y/N]", stripped, ctxName)
	}
	return fmt.Sprintf("Delete secret %s in context %s? It can be recovered for %d days after deletion [y/N]", stripped, ctxName, aws.SecretsManagerRecoveryWindowDays)
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
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
