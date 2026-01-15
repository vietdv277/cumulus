package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Global flags
	profile string
	region  string
)

var rootCmd = &cobra.Command{
	Use:   "cml",
	Short: "Culumus - Multi-cloud CLI tool for AWS and GCP",
	Long: `Cumulus is a command-line interface (CLI) tool designed to manage and interact with multiple cloud service providers, specifically Amazon Web Services (AWS) and Google Cloud Platform (GCP). It provides a unified interface to perform various cloud operations across these platforms.

Examples:
  cml ec2 ls              # List running EC2 instances
  cml ec2 ssh             # Interactive SSH via SSM
  cml profile             # Switch AWS profile`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	//Global persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVarP(&region, "region", "r", "", "AWS region to use")

	// Bind flags to viper
	_ = viper.BindPFlag("profile", rootCmd.PersistentFlags().Lookup("profile"))
	_ = viper.BindPFlag("region", rootCmd.PersistentFlags().Lookup("region"))
}

func initConfig() {
	// Read from environment variables
	viper.SetEnvPrefix("CML")
	viper.AutomaticEnv()

	// Use AWS_PROFILE if --profile not specified
	if profile == "" {
		profile = os.Getenv("AWS_PROFILE")
	}

	// Use AWS_REGION if --region not specified
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
	}
}

// GetProfile returns the AWS profile
func GetProfile() string {
	return profile
}

// GetRegion returns the AWS region
func GetRegion() string {
	return region
}
