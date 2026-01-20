package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/spf13/cobra"
	internalConfig "github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var ssmCmd = &cobra.Command{
	Use:   "ssm",
	Short: "AWS Systems Manager commands",
	Long: `AWS Systems Manager (SSM) commands for parameter store operations.

Examples:
  cml aws ssm param list /app/
  cml aws ssm param get /app/db-password
  cml aws ssm param set /app/new-param "value"`,
}

var ssmParamCmd = &cobra.Command{
	Use:   "param",
	Short: "Parameter Store operations",
	Long: `Manage SSM Parameter Store parameters.

Examples:
  cml aws ssm param list /app/
  cml aws ssm param get /app/db-password`,
}

var ssmParamListCmd = &cobra.Command{
	Use:   "list [prefix]",
	Short: "List parameters",
	Long: `List SSM parameters with optional prefix filter.

Examples:
  cml aws ssm param list
  cml aws ssm param list /app/
  cml aws ssm param list /production/`,
	RunE: runSSMParamList,
}

var ssmParamGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get parameter value",
	Long: `Get the value of an SSM parameter.

Examples:
  cml aws ssm param get /app/db-password
  cml aws ssm param get /app/config --decode`,
	Args: cobra.ExactArgs(1),
	RunE: runSSMParamGet,
}

var ssmParamSetCmd = &cobra.Command{
	Use:   "set <name> <value>",
	Short: "Set parameter value",
	Long: `Create or update an SSM parameter.

Examples:
  cml aws ssm param set /app/new-param "value"
  cml aws ssm param set /app/secret "secret-value" --secure`,
	Args: cobra.ExactArgs(2),
	RunE: runSSMParamSet,
}

var (
	ssmParamDecode bool
	ssmParamSecure bool
)

func init() {
	ssmCmd.AddCommand(ssmParamCmd)
	ssmParamCmd.AddCommand(ssmParamListCmd)
	ssmParamCmd.AddCommand(ssmParamGetCmd)
	ssmParamCmd.AddCommand(ssmParamSetCmd)

	ssmParamGetCmd.Flags().BoolVar(&ssmParamDecode, "decode", false, "Base64 decode the value")
	ssmParamSetCmd.Flags().BoolVar(&ssmParamSecure, "secure", false, "Create as SecureString")
}

func getSSMClient() (*ssm.Client, error) {
	ctx := context.Background()

	// Get current context
	ctxConfig, _, err := internalConfig.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	if ctxConfig == nil || ctxConfig.Provider != "aws" {
		return nil, fmt.Errorf("current context is not AWS. Use 'cml use aws:<context>'")
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
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return ssm.NewFromConfig(cfg), nil
}

func runSSMParamList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getSSMClient()
	if err != nil {
		return err
	}

	prefix := ""
	if len(args) > 0 {
		prefix = args[0]
	}

	// List parameters
	input := &ssm.DescribeParametersInput{}

	if prefix != "" {
		input.ParameterFilters = []ssmTypes.ParameterStringFilter{
			{
				Key:    stringPtr("Name"),
				Option: stringPtr("BeginsWith"),
				Values: []string{prefix},
			},
		}
	}

	paginator := ssm.NewDescribeParametersPaginator(client, input)

	var params []paramInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list parameters: %w", err)
		}

		for _, p := range page.Parameters {
			lastMod := ""
			if p.LastModifiedDate != nil {
				lastMod = p.LastModifiedDate.Format("2006-01-02 15:04")
			}
			params = append(params, paramInfo{
				Name:         deref(p.Name),
				Type:         string(p.Type),
				LastModified: lastMod,
			})
		}
	}

	if len(params) == 0 {
		fmt.Println("No parameters found")
		return nil
	}

	// Print table
	printParamTable(params)

	return nil
}

func runSSMParamGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getSSMClient()
	if err != nil {
		return err
	}

	name := args[0]

	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter: %w", err)
	}

	value := deref(output.Parameter.Value)

	if ssmParamDecode {
		// Base64 decode
		decoded, err := base64Decode(value)
		if err != nil {
			return fmt.Errorf("failed to decode value: %w", err)
		}
		value = decoded
	}

	fmt.Println(value)
	return nil
}

func runSSMParamSet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := getSSMClient()
	if err != nil {
		return err
	}

	name := args[0]
	value := args[1]

	paramType := ssmTypes.ParameterTypeString
	if ssmParamSecure {
		paramType = ssmTypes.ParameterTypeSecureString
	}

	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      &name,
		Value:     &value,
		Type:      paramType,
		Overwrite: boolPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	fmt.Printf("Parameter set: %s\n", name)
	return nil
}

type paramInfo struct {
	Name         string
	Type         string
	LastModified string
}

func printParamTable(params []paramInfo) {
	headers := []string{"Name", "Type", "Last Modified"}
	widths := []int{50, 15, 18}

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
		cell := " " + padStr(h, widths[i]) + " "
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
	for _, p := range params {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell := " " + padStr(p.Name, widths[0]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padStr(p.Type, widths[1]) + " "
		sb.WriteString(ui.TypeStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padStr(p.LastModified, widths[2]) + " "
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
	fmt.Printf("  %d parameters\n", len(params))
}

func padStr(s string, width int) string {
	if len(s) >= width {
		if width > 3 {
			return s[:width-3] + "..."
		}
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func base64Decode(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
