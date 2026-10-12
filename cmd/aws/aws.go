package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	internalConfig "github.com/vietdv277/cumulus/internal/config"
)

// awsContextFlag holds the persistent -c/--context value.
var awsContextFlag string

// AWSCmd is the root command for AWS-specific operations
var AWSCmd = &cobra.Command{
	Use:   "aws",
	Short: "AWS-specific commands",
	Long: `AWS-specific commands for features without cross-provider equivalents.

Use --context <name-or-alias> to operate in a specific context instead of the
current one.

Examples:
  cml aws ssm param list /app/
  cml aws ssm param get /app/db-password
  cml aws iam whoami`,
}

func init() {
	AWSCmd.AddCommand(ssmCmd)
	AWSCmd.AddCommand(iamCmd)

	AWSCmd.PersistentFlags().StringVarP(&awsContextFlag, "context", "c", "", "Use specific context")
}

// resolveAWSContext resolves the -c/--context flag and requires an AWS context.
func resolveAWSContext() (*internalConfig.Context, string, error) {
	ctxConfig, ctxName, err := internalConfig.ResolveContext(awsContextFlag)
	if err != nil {
		return nil, "", err
	}
	if ctxConfig.Provider != "aws" {
		return nil, "", fmt.Errorf("context %s is a %s context — aws commands need an AWS context", ctxName, ctxConfig.Provider)
	}
	return ctxConfig, ctxName, nil
}

// loadAWSConfig builds an AWS SDK config from the context's profile/region.
func loadAWSConfig(ctx context.Context, ctxConfig *internalConfig.Context) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	if ctxConfig.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(ctxConfig.Profile))
	}
	if ctxConfig.Region != "" {
		opts = append(opts, awsconfig.WithRegion(ctxConfig.Region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return cfg, nil
}
