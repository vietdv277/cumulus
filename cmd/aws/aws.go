package aws

import (
	"github.com/spf13/cobra"
)

// AWSCmd is the root command for AWS-specific operations
var AWSCmd = &cobra.Command{
	Use:   "aws",
	Short: "AWS-specific commands",
	Long: `AWS-specific commands for features without cross-provider equivalents.

Examples:
  cml aws ssm param list /app/
  cml aws ssm param get /app/db-password
  cml aws iam whoami`,
}

func init() {
	AWSCmd.AddCommand(ssmCmd)
	AWSCmd.AddCommand(iamCmd)
}
