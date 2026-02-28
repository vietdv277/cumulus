package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	awscmd "github.com/vietdv277/cumulus/cmd/aws"
	"github.com/vietdv277/cumulus/internal/config"
)

var (
	// Global flags
	profile string
	region  string
)

var rootCmd = &cobra.Command{
	Use:   "cml",
	Short: "Cumulus - Multi-cloud CLI tool for AWS and GCP",
	Long: `Cumulus is a command-line interface (CLI) tool designed to manage and interact
with multiple cloud service providers. It provides a unified, context-aware interface
for managing cloud resources.

Context-Aware Commands:
  cml use aws:prod           # Switch to AWS production context
  cml status                 # Show current context and auth status
  cml contexts               # List all configured contexts

Unified Resource Commands:
  cml vm list                # List VMs in current context
  cml vm connect <name>      # SSH/SSM to a VM
  cml vm tunnel <name> 3306  # Port forward to a VM

Provider-Specific Commands:
  cml aws ssm param list     # List SSM parameters
  cml aws iam whoami         # Show AWS identity

Legacy Commands (still available):
  cml ec2 ls                 # List EC2 instances
  cml profile                # Manage profiles`,
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

	// Add provider-specific commands
	rootCmd.AddCommand(awscmd.AWSCmd)
}

func initConfig() {
	// Migrate from legacy formats if needed (oldest first)
	config.MigrateFromMacOSConfig()   // ~/Library/Application Support/cml/ → XDG path
	config.MigrateFromOldConfig()     // ~/.cml/config.yaml → XDG path
	config.MigrateFromDotFileConfig() // ~/.cml.yaml        → XDG path

	// Read from environment variables
	viper.SetEnvPrefix("CML")
	viper.AutomaticEnv()

	// Priority for profile: --profile flag > ~/.config/cml/config.yaml > AWS_PROFILE env
	if profile == "" {
		if saved := config.GetSavedProfile(); saved != "" {
			profile = saved
		} else {
			profile = os.Getenv("AWS_PROFILE")
		}
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
