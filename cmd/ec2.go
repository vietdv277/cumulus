package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

var ec2Cmd = &cobra.Command{
	Use: "ec2",
	Short: "Manage EC2 instances",
	Long: `Perform various operations on EC2 instances such as listing, starting, stopping, and terminating instances.`,
}

var ec2LsCmd = &cobra.Command{
	Use: "ls",
	Short: "List running EC2 instances",
	Long: `List all running EC2 instances with optional filters.

	Examples:
	cml ec2 ls # List all running instances
	cml ec2 ls --name kong # Filter by name pattern
	cml ec2 ls --asg my-asg # Filter by ASG name
	cme ec2 ls --all # Include stopped instances`,

	RunE: runEC2List,
}

var (
	// ec2 flags
	namePattern string
	asgName     string
	showAll     bool
)

func init() {
	// Add ec2 commmand to root
	rootCmd.AddCommand(ec2Cmd)

	// Add subcommands to ec2
	rootCmd.AddCommand(ec2LsCmd)

	// FLags for ec2 ls
	ec2LsCmd.Flags().StringVar(&namePattern, "name", "n", "Filter instances by name pattern")
	ec2LsCmd.Flags().StringVar(&asgName, "asg", "a", "Filter instances by Auto Scaling Group name")
	ec2LsCmd.Flags().BoolVar(&showAll, "all", false, "Show all instances including stopped ones")
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
func truncate (s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
