package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/ui"
)

var asgCmd = &cobra.Command{
	Use:   "asg",
	Short: "Manage Auto Scaling Groups",
	Long:  `Perform various operations on Auto Scaling Groups such as listing, describing, scaling, and refreshing instances.`,
}

var asgLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List Auto Scaling Groups",
	Long: `List all Auto Scaling Groups with capacity and health information.

Examples:
  cml asg ls              # List all ASGs
  cml asg ls --name web   # Filter by name pattern`,
	RunE: runASGList,
}

var asgDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Describe an Auto Scaling Group",
	Long: `Show detailed information about an Auto Scaling Group including its instances.

If no name is provided, an interactive selector will be shown.

Examples:
  cml asg describe my-asg    # Describe specific ASG
  cml asg describe           # Interactive selector`,
	RunE: runASGDescribe,
}

var asgScaleCmd = &cobra.Command{
	Use:   "scale <name>",
	Short: "Scale an Auto Scaling Group",
	Long: `Update the capacity settings of an Auto Scaling Group.

At least one of --desired, --min, or --max must be specified.

Examples:
  cml asg scale my-asg --desired 5
  cml asg scale my-asg --min 2 --max 10
  cml asg scale my-asg --desired 3 --min 1 --max 5`,
	Args: cobra.ExactArgs(1),
	RunE: runASGScale,
}

var asgRefreshCmd = &cobra.Command{
	Use:   "refresh <name>",
	Short: "Start instance refresh",
	Long: `Start a rolling instance refresh for an Auto Scaling Group.

This replaces instances with new ones using the current launch template.

Examples:
  cml asg refresh my-asg
  cml asg refresh my-asg --min-healthy 80`,
	Args: cobra.ExactArgs(1),
	RunE: runASGRefresh,
}

var (
	// asg ls flags
	asgNamePattern string

	// asg scale flags
	scaleDesired int
	scaleMin     int
	scaleMax     int

	// asg refresh flags
	refreshMinHealthy int
)

func init() {
	rootCmd.AddCommand(asgCmd)

	asgCmd.AddCommand(asgLsCmd)
	asgCmd.AddCommand(asgDescribeCmd)
	asgCmd.AddCommand(asgScaleCmd)
	asgCmd.AddCommand(asgRefreshCmd)

	// Flags for asg ls
	asgLsCmd.Flags().StringVar(&asgNamePattern, "name", "", "Filter ASGs by name pattern")

	// Flags for asg scale
	asgScaleCmd.Flags().IntVar(&scaleDesired, "desired", -1, "Desired capacity")
	asgScaleCmd.Flags().IntVar(&scaleMin, "min", -1, "Minimum size")
	asgScaleCmd.Flags().IntVar(&scaleMax, "max", -1, "Maximum size")

	// Flags for asg refresh
	asgRefreshCmd.Flags().IntVar(&refreshMinHealthy, "min-healthy", 90, "Minimum healthy percentage during refresh")
}

func runASGList(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	input := &aws.ListASGInput{
		NamePattern: asgNamePattern,
	}

	groups, err := client.ListAutoScalingGroups(input)
	if err != nil {
		return fmt.Errorf("failed to list Auto Scaling Groups: %w", err)
	}

	if len(groups) == 0 {
		fmt.Println("No Auto Scaling Groups found")
		return nil
	}

	ui.PrintASGTable(groups)
	return nil
}

func runASGDescribe(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var asgName string

	if len(args) > 0 {
		asgName = args[0]
	} else {
		// Interactive selection
		groups, err := client.ListAutoScalingGroups(nil)
		if err != nil {
			return fmt.Errorf("failed to list Auto Scaling Groups: %w", err)
		}

		if len(groups) == 0 {
			fmt.Println("No Auto Scaling Groups found")
			return nil
		}

		selected, err := ui.SelectASG(groups)
		if err != nil {
			return err
		}
		asgName = selected.Name
	}

	asg, err := client.DescribeAutoScalingGroup(asgName)
	if err != nil {
		return fmt.Errorf("failed to describe ASG: %w", err)
	}

	ui.PrintASGDetails(asg)
	return nil
}

func runASGScale(cmd *cobra.Command, args []string) error {
	asgName := args[0]

	// Validate that at least one flag is provided
	if scaleDesired < 0 && scaleMin < 0 && scaleMax < 0 {
		return fmt.Errorf("at least one of --desired, --min, or --max must be specified")
	}

	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get current ASG info for confirmation
	asg, err := client.DescribeAutoScalingGroup(asgName)
	if err != nil {
		return fmt.Errorf("failed to describe ASG: %w", err)
	}

	// Show what will change
	fmt.Printf("Auto Scaling Group: %s\n", asgName)
	fmt.Printf("Current: Desired=%d, Min=%d, Max=%d\n", asg.DesiredCapacity, asg.MinSize, asg.MaxSize)

	newDesired := asg.DesiredCapacity
	newMin := asg.MinSize
	newMax := asg.MaxSize

	if scaleDesired >= 0 {
		newDesired = scaleDesired
	}
	if scaleMin >= 0 {
		newMin = scaleMin
	}
	if scaleMax >= 0 {
		newMax = scaleMax
	}

	fmt.Printf("New:     Desired=%d, Min=%d, Max=%d\n", newDesired, newMin, newMax)
	fmt.Print("\nProceed with scaling? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Scaling cancelled")
		return nil
	}

	input := &aws.UpdateASGInput{
		Name: asgName,
	}

	if scaleDesired >= 0 {
		input.DesiredCapacity = &scaleDesired
	}
	if scaleMin >= 0 {
		input.MinSize = &scaleMin
	}
	if scaleMax >= 0 {
		input.MaxSize = &scaleMax
	}

	if err := client.UpdateAutoScalingGroup(input); err != nil {
		return fmt.Errorf("failed to scale ASG: %w", err)
	}

	fmt.Printf("Successfully updated Auto Scaling Group %s\n", asgName)
	return nil
}

func runASGRefresh(cmd *cobra.Command, args []string) error {
	asgName := args[0]

	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get current ASG info
	asg, err := client.DescribeAutoScalingGroup(asgName)
	if err != nil {
		return fmt.Errorf("failed to describe ASG: %w", err)
	}

	fmt.Printf("Auto Scaling Group: %s\n", asgName)
	fmt.Printf("Instance Count: %d\n", asg.InstanceCount)
	fmt.Printf("Min Healthy Percentage: %d%%\n", refreshMinHealthy)
	fmt.Print("\nThis will start a rolling refresh of all instances. Proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Refresh cancelled")
		return nil
	}

	refreshID, err := client.StartInstanceRefresh(&aws.RefreshInput{
		Name:              asgName,
		MinHealthyPercent: refreshMinHealthy,
	})
	if err != nil {
		return fmt.Errorf("failed to start instance refresh: %w", err)
	}

	fmt.Printf("Instance refresh started: %s\n", refreshID)
	fmt.Println("Use AWS Console or CLI to monitor progress")
	return nil
}
