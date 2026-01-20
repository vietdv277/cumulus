package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"
	internalConfig "github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var iamCmd = &cobra.Command{
	Use:   "iam",
	Short: "AWS IAM commands",
	Long: `AWS IAM commands for identity operations.

Examples:
  cml aws iam whoami`,
}

var iamWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current AWS identity",
	Long: `Display the current AWS caller identity.

Equivalent to 'aws sts get-caller-identity'.

Examples:
  cml aws iam whoami`,
	RunE: runIAMWhoami,
}

func init() {
	iamCmd.AddCommand(iamWhoamiCmd)
}

func runIAMWhoami(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get current context
	ctxConfig, ctxName, err := internalConfig.GetCurrentContext()
	if err != nil {
		return err
	}

	if ctxConfig == nil || ctxConfig.Provider != "aws" {
		return fmt.Errorf("current context is not AWS. Use 'cml use aws:<context>'")
	}

	var configOpts []func(*config.LoadOptions) error

	if ctxConfig.Profile != "" {
		configOpts = append(configOpts, config.WithSharedConfigProfile(ctxConfig.Profile))
	}

	if ctxConfig.Region != "" {
		configOpts = append(configOpts, config.WithRegion(ctxConfig.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)

	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("AWS Identity"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  Context: %s\n", ui.AWSStyle.Render(ctxName))
	fmt.Printf("  Profile: %s\n", ctxConfig.Profile)
	fmt.Println()
	fmt.Printf("  Account: %s\n", derefStr(output.Account))
	fmt.Printf("  UserID:  %s\n", derefStr(output.UserId))
	fmt.Printf("  ARN:     %s\n", ui.MutedStyle.Render(derefStr(output.Arn)))

	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
