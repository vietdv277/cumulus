package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/output"
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

// identityOutput is the stable json/yaml shape of `cml aws iam whoami`.
type identityOutput struct {
	Context string `json:"context" yaml:"context"`
	Profile string `json:"profile" yaml:"profile"`
	Account string `json:"account" yaml:"account"`
	UserID  string `json:"user_id" yaml:"user_id"`
	ARN     string `json:"arn" yaml:"arn"`
}

func runIAMWhoami(cmd *cobra.Command, args []string) error {
	out, err := output.Format(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Get the resolved context (flag wins over current; legacy flags rejected)
	ctxConfig, ctxName, err := resolveAWSContext(cmd)
	if err != nil {
		return err
	}

	cfg, err := loadAWSConfig(ctx, ctxConfig)
	if err != nil {
		return err
	}

	stsClient := sts.NewFromConfig(cfg)

	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}

	if out != output.Table {
		doc := identityOutput{
			Context: ctxName,
			Profile: ctxConfig.Profile,
			Account: derefStr(identity.Account),
			UserID:  derefStr(identity.UserId),
			ARN:     derefStr(identity.Arn),
		}
		return output.Render(os.Stdout, out, doc, nil)
	}

	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("AWS Identity"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  Context: %s\n", ui.AWSStyle.Render(ctxName))
	fmt.Printf("  Profile: %s\n", ctxConfig.Profile)
	fmt.Println()
	fmt.Printf("  Account: %s\n", derefStr(identity.Account))
	fmt.Printf("  UserID:  %s\n", derefStr(identity.UserId))
	fmt.Printf("  ARN:     %s\n", ui.MutedStyle.Render(derefStr(identity.Arn)))

	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
