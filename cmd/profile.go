package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage AWS profiles",
	Long: `Manage AWS profiles for the CLI.

When run without subcommands, shows an interactive selector to choose a profile.

Examples:
  cml profile                    # Interactive profile selector
  cml profile ls                 # List all available profiles
  cml profile set my-profile     # Set a specific profile`,
	RunE: runProfileInteractive,
}

var profileLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available AWS profiles",
	Long: `List all available AWS profiles from ~/.aws/credentials and ~/.aws/config.

Examples:
  cml profile ls`,
	RunE: runProfileList,
}

var profileSetCmd = &cobra.Command{
	Use:   "set <profile-name>",
	Short: "Set the active AWS profile",
	Long: `Set a specific AWS profile as active.

The profile will be saved to ~/.config/cml/config.yaml and used by future cml commands.

Examples:
  cml profile set my-profile
  cml profile set production`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileSet,
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileLsCmd)
	profileCmd.AddCommand(profileSetCmd)
}

func runProfileInteractive(cmd *cobra.Command, args []string) error {
	profiles, err := aws.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No AWS profiles found")
		fmt.Println("Create profiles in ~/.aws/credentials or ~/.aws/config")
		return nil
	}

	// Get current active profile
	activeProfile := getActiveProfile()

	// Show interactive selector
	selected, err := ui.SelectProfile(profiles, activeProfile)
	if err != nil {
		return err
	}

	// Save to config
	if err := config.SetProfile(selected.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save profile to config: %v\n", err)
	}

	// Print success message and export command
	fmt.Printf("\nProfile set to: %s\n", selected.Name)
	fmt.Printf("Saved to: %s\n\n", config.GetConfigPath())
	fmt.Println("To use this profile in your current shell, run:")
	fmt.Printf("  export AWS_PROFILE=%s\n", selected.Name)

	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	profiles, err := aws.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No AWS profiles found")
		fmt.Println("Create profiles in ~/.aws/credentials or ~/.aws/config")
		return nil
	}

	// Get current active profile
	activeProfile := getActiveProfile()

	ui.PrintProfileTable(profiles, activeProfile)
	return nil
}

func runProfileSet(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	// Validate profile exists
	if !aws.ValidateProfile(profileName) {
		return fmt.Errorf("profile %q not found", profileName)
	}

	// Save to config
	if err := config.SetProfile(profileName); err != nil {
		return fmt.Errorf("failed to save profile: %w", err)
	}

	fmt.Printf("Profile set to: %s\n", profileName)
	fmt.Printf("Saved to: %s\n\n", config.GetConfigPath())
	fmt.Println("To use this profile in your current shell, run:")
	fmt.Printf("  export AWS_PROFILE=%s\n", profileName)

	return nil
}

// getActiveProfile returns the currently active profile
func getActiveProfile() string {
	// Priority: --profile flag > config file > AWS_PROFILE env
	if profile != "" {
		return profile
	}

	if saved := config.GetSavedProfile(); saved != "" {
		return saved
	}

	return os.Getenv("AWS_PROFILE")
}
