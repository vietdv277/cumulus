package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/ui"
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

var ec2TunnelCmd = &cobra.Command{
	Use:   "tunnel [instance-id]",
	Short: "Create SSH tunnel to an EC2 instance via SSM",
	Long: `Create an SSH tunnel (port forwarding) to an EC2 instance using AWS SSM.
If no instance ID is provided, an interactive selector will be shown.

The tunnel forwards a local port to a remote port on the EC2 instance,
allowing you to access services running on the instance (databases, web servers, etc.).

Examples:
  cml ec2 tunnel                              # Interactive, prompts for ports
  cml ec2 tunnel -L 3306:3306                 # Forward local 3306 to remote 3306
  cml ec2 tunnel -L 5432:5432 --name db       # PostgreSQL tunnel to instance matching "db"
  cml ec2 tunnel --service mysql              # Use preset for MySQL (3306)
  cml ec2 tunnel --service postgres           # Use preset for PostgreSQL (5432)
  cml ec2 tunnel --service redis              # Use preset for Redis (6379)
  cml ec2 tunnel -L 8080:80                   # Forward local 8080 to remote 80

Available service presets:
  mysql      - 3306:3306
  postgres   - 5432:5432
  redis      - 6379:6379
  mongodb    - 27017:27017
  ssh        - 2222:22
  http       - 8080:80
  https      - 8443:443`,
	RunE: runEC2Tunnel,
}

var (
	// ec2 ls flags
	namePattern string
	asgName     string
	showAll     bool

	// ec2 ssh flags
	sshNamePattern string
	sshASGName     string

	// ec2 tunnel flags
	tunnelNamePattern string
	tunnelASGName     string
	tunnelLocalPort   string // -L flag like SSH: localPort:remotePort
	tunnelService     string // preset service name
)

// Service presets for common port forwarding scenarios
var servicePresets = map[string][2]int{
	"mysql":    {3306, 3306},
	"postgres": {5432, 5432},
	"redis":    {6379, 6379},
	"mongodb":  {27017, 27017},
	"ssh":      {2222, 22},
	"http":     {8080, 80},
	"https":    {8443, 443},
}

func init() {
	// Add ec2 command to root
	rootCmd.AddCommand(ec2Cmd)

	// Add subcommands to ec2
	ec2Cmd.AddCommand(ec2LsCmd)
	ec2Cmd.AddCommand(ec2SSHCmd)
	ec2Cmd.AddCommand(ec2TunnelCmd)

	// Flags for ec2 ls
	ec2LsCmd.Flags().StringVar(&namePattern, "name", "", "Filter instances by name pattern")
	ec2LsCmd.Flags().StringVar(&asgName, "asg", "", "Filter instances by Auto Scaling Group name")
	ec2LsCmd.Flags().BoolVar(&showAll, "all", false, "Show all instances including stopped ones")

	// Flags for ec2 ssh
	ec2SSHCmd.Flags().StringVar(&sshNamePattern, "name", "", "Filter instances by name pattern")
	ec2SSHCmd.Flags().StringVar(&sshASGName, "asg", "", "Filter instances by Auto Scaling Group name")

	// Flags for ec2 tunnel
	ec2TunnelCmd.Flags().StringVar(&tunnelNamePattern, "name", "", "Filter instances by name pattern")
	ec2TunnelCmd.Flags().StringVar(&tunnelASGName, "asg", "", "Filter instances by Auto Scaling Group name")
	ec2TunnelCmd.Flags().StringVarP(&tunnelLocalPort, "local", "L", "", "Port forwarding in format localPort:remotePort (e.g., 3306:3306)")
	ec2TunnelCmd.Flags().StringVar(&tunnelService, "service", "", "Service preset (mysql, postgres, redis, mongodb, ssh, http, https)")
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
	ui.PrintInstanceTable(instances)

	return nil
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

func runEC2Tunnel(cmd *cobra.Command, args []string) error {
	// Create AWS client
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var instanceID string

	// Check if instance ID provided as argument
	if len(args) > 0 {
		instanceID = args[0]
	} else {
		// Build input for listing running instances only
		input := &aws.ListInstanceInput{
			NamePattern: tunnelNamePattern,
			ASGName:     tunnelASGName,
			States:      []string{"running"},
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
		instanceID = selected.ID
	}

	// Determine ports
	var localPort, remotePort int

	if tunnelService != "" {
		// Use service preset
		preset, ok := servicePresets[strings.ToLower(tunnelService)]
		if !ok {
			return fmt.Errorf("unknown service preset: %s (available: mysql, postgres, redis, mongodb, ssh, http, https)", tunnelService)
		}
		localPort = preset[0]
		remotePort = preset[1]
	} else if tunnelLocalPort != "" {
		// Parse -L flag: localPort:remotePort
		parts := strings.Split(tunnelLocalPort, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid port format: %s (expected localPort:remotePort)", tunnelLocalPort)
		}

		localPort, err = strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid local port: %s", parts[0])
		}

		remotePort, err = strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid remote port: %s", parts[1])
		}
	} else {
		return fmt.Errorf("please specify port forwarding with -L localPort:remotePort or --service <name>")
	}

	// Start port forwarding session
	fmt.Printf("Starting tunnel to %s: localhost:%d -> instance:%d\n", instanceID, localPort, remotePort)
	fmt.Println("Press Ctrl+C to close the tunnel")

	return startSSMPortForward(instanceID, localPort, remotePort, client.Profile(), client.Region())
}

// startSSMPortForward starts an AWS SSM port forwarding session
func startSSMPortForward(instanceID string, localPort, remotePort int, profile, region string) error {
	// Build the parameters JSON
	params := map[string][]string{
		"portNumber":      {strconv.Itoa(remotePort)},
		"localPortNumber": {strconv.Itoa(localPort)},
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", string(paramsJSON),
	}

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
