package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/ui"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

var ec2Cmd = &cobra.Command{
	Use: "ec2",
	Short: "Manage EC2 instances",
	Long: `Perform various operations on EC2 instances such as listing, starting, stopping, and terminating instances.`,
}

var ec2LsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List running EC2 instances",
	Long: `List all running EC2 instances with optional filters.

Examples:
  cml ec2 ls              # List all running instances
  cml ec2 ls --name kong  # Filter by name pattern
  cml ec2 ls --asg my-asg # Filter by ASG name
  cml ec2 ls --all        # Include stopped instances`,
	RunE: runEC2List,
}

var ec2SSHCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Start SSM session to an EC2 instance",
	Long: `Interactively select a running EC2 instance and start an SSM session.

Examples:
  cml ec2 ssh                        # Select from all running instances
  cml ec2 ssh --name web             # Filter by name pattern
  cml ec2 ssh --asg my-asg           # Filter by ASG name
  cml ec2 ssh -p production          # Use specific AWS profile`,
	RunE: runEC2SSH,
}

var (
	// ec2 ls flags
	namePattern string
	asgName     string
	showAll     bool

	// ec2 ssh flags
	sshNamePattern string
	sshASGName     string
)

func init() {
	// Add ec2 command to root
	rootCmd.AddCommand(ec2Cmd)

	// Add subcommands to ec2
	ec2Cmd.AddCommand(ec2LsCmd)
	ec2Cmd.AddCommand(ec2SSHCmd)

	// Flags for ec2 ls
	ec2LsCmd.Flags().StringVar(&namePattern, "name", "", "Filter instances by name pattern")
	ec2LsCmd.Flags().StringVar(&asgName, "asg", "", "Filter instances by Auto Scaling Group name")
	ec2LsCmd.Flags().BoolVar(&showAll, "all", false, "Show all instances including stopped ones")

	// Flags for ec2 ssh (reuse name and asg filters)
	ec2SSHCmd.Flags().StringVar(&sshNamePattern, "name", "", "Filter instances by name pattern")
	ec2SSHCmd.Flags().StringVar(&sshASGName, "asg", "", "Filter instances by Auto Scaling Group name")
}

func runEC2List(cmd *cobra.Command, args []string) error {
	// Create AWS client
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Build input
	input := &aws.ListInstanceInput{
		NamePattern: namePattern,
		ASGName: asgName,
	}

	if showAll {
		input.States = []string{"pending", "running", "stopping", "stopped"}
	}

	// List instances
	instances, err := client.ListInstances(input)
	if err != nil {
		return fmt.Errorf("failed to list EC2 instances: %w", err)
	}

	if len(instances) == 0 {
		fmt.Println("No EC2 instances found")
		return nil
	}

	// Print results
	printInstanceTable(instances)

	return nil
}

func printInstanceTable(instances []pkgtypes.Instance) {
	// Create styled table
	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	tbl := table.New("ID", "Name", "Private IP", "State", "Type", "AZ", "ASG")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
	tbl.WithWriter(os.Stdout)

	for _, inst := range instances {
		tbl.AddRow(
			inst.ID,
			truncate(inst.Name, 30),
			inst.PrivateIP,
			inst.State,
			inst.Type,
			inst.AZ,
			truncate(inst.ASG, 25),
		)
	}

	tbl.Print()
	fmt.Printf("\nTotal: %d instances\n", len(instances))
}

// truncate shortens a string with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func runEC2SSH(cmd *cobra.Command, args []string) error {
	// Create AWS client
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Build input for listing running instances only
	input := &aws.ListInstanceInput{
		NamePattern: sshNamePattern,
		ASGName:     sshASGName,
		States:      []string{"running"}, // Only running instances can be SSM'd
	}

	// List instances
	instances, err := client.ListInstances(input)
	if err != nil {
		return fmt.Errorf("failed to list EC2 instances: %w", err)
	}

	if len(instances) == 0 {
		fmt.Println("No running EC2 instances found")
		return nil
	}

	// Show interactive selector
	selected, err := ui.SelectInstance(instances)
	if err != nil {
		return err
	}

	// Start SSM session
	fmt.Printf("Starting SSM session to %s (%s)...\n", selected.Name, selected.ID)
	return startSSMSession(selected.ID, client.Profile(), client.Region())
}

// startSSMSession starts an AWS SSM session using the AWS CLI
func startSSMSession(instanceID, profile, region string) error {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}

	if region != "" {
		args = append(args, "--region", region)
	}

	ssmCmd := exec.Command("aws", args...)
	ssmCmd.Stdin = os.Stdin
	ssmCmd.Stdout = os.Stdout
	ssmCmd.Stderr = os.Stderr

	return ssmCmd.Run()
}
