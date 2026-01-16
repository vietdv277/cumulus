package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/ui"
)

var lbCmd = &cobra.Command{
	Use:   "lb",
	Short: "Manage Load Balancers",
	Long:  `Perform various operations on AWS Load Balancers (ALB/NLB) such as listing, describing, and viewing targets.`,
}

var lbLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all load balancers",
	Long: `List all load balancers (ALB/NLB) with their type, scheme, state, and DNS name.

Examples:
  cml lb ls              # List all load balancers
  cml lb ls -p prod      # List LBs using production profile`,
	RunE: runLBList,
}

var lbDescribeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show detailed load balancer information",
	Long: `Show detailed information about a load balancer including listeners and target groups.
If no name is provided, an interactive selector will be shown.

Examples:
  cml lb describe                  # Interactive LB selector
  cml lb describe my-alb           # Describe specific LB`,
	RunE: runLBDescribe,
}

var lbTargetsCmd = &cobra.Command{
	Use:   "targets [name]",
	Short: "List targets behind a load balancer",
	Long: `List all targets/instances behind a load balancer with their health status.
If no name is provided, an interactive selector will be shown.

Examples:
  cml lb targets                   # Interactive LB selector
  cml lb targets my-alb            # List targets for specific LB`,
	RunE: runLBTargets,
}

func init() {
	rootCmd.AddCommand(lbCmd)

	lbCmd.AddCommand(lbLsCmd)
	lbCmd.AddCommand(lbDescribeCmd)
	lbCmd.AddCommand(lbTargetsCmd)
}

func runLBList(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	lbs, err := client.ListLoadBalancers()
	if err != nil {
		return fmt.Errorf("failed to list load balancers: %w", err)
	}

	if len(lbs) == 0 {
		fmt.Println("No load balancers found")
		return nil
	}

	ui.PrintLBTable(lbs)
	return nil
}

func runLBDescribe(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var lbName string
	var lbARN string

	if len(args) > 0 {
		lbName = args[0]
		// Get the LB to get its ARN
		lb, err := client.GetLoadBalancerByName(lbName)
		if err != nil {
			return fmt.Errorf("failed to get load balancer: %w", err)
		}
		if lb == nil {
			return fmt.Errorf("load balancer %s not found", lbName)
		}
		lbARN = lb.ARN
	} else {
		// Interactive selector
		lbs, err := client.ListLoadBalancers()
		if err != nil {
			return fmt.Errorf("failed to list load balancers: %w", err)
		}

		selected, err := ui.SelectLoadBalancer(lbs)
		if err != nil {
			return err
		}
		lbName = selected.Name
		lbARN = selected.ARN
	}

	// Get LB details
	lb, err := client.GetLoadBalancerByName(lbName)
	if err != nil {
		return fmt.Errorf("failed to describe load balancer: %w", err)
	}

	if lb == nil {
		return fmt.Errorf("load balancer %s not found", lbName)
	}

	// Print LB details
	fmt.Println()
	fmt.Printf("Load Balancer: %s\n", lb.Name)
	fmt.Printf("  Type:      %s\n", lb.Type)
	fmt.Printf("  Scheme:    %s\n", lb.Scheme)
	fmt.Printf("  State:     %s\n", lb.State)
	fmt.Printf("  DNS:       %s\n", lb.DNSName)
	fmt.Printf("  VPC:       %s\n", lb.VPCID)
	fmt.Printf("  AZs:       %s\n", strings.Join(lb.AZs, ", "))
	fmt.Printf("  Created:   %s\n", lb.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Get listeners
	listeners, err := client.ListListeners(lbARN)
	if err != nil {
		return fmt.Errorf("failed to list listeners: %w", err)
	}

	if len(listeners) > 0 {
		fmt.Println("Listeners:")
		for _, l := range listeners {
			fmt.Printf("  - %s:%d\n", l.Protocol, l.Port)
		}
		fmt.Println()
	}

	// Get target groups
	tgs, err := client.ListTargetGroups(lbARN)
	if err != nil {
		return fmt.Errorf("failed to list target groups: %w", err)
	}

	if len(tgs) > 0 {
		fmt.Println("Target Groups:")
		ui.PrintTargetGroupTable(tgs)
	} else {
		fmt.Println("No target groups found")
	}

	return nil
}

func runLBTargets(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var lbName string
	var lbARN string

	if len(args) > 0 {
		lbName = args[0]
		// Get the LB to get its ARN
		lb, err := client.GetLoadBalancerByName(lbName)
		if err != nil {
			return fmt.Errorf("failed to get load balancer: %w", err)
		}
		if lb == nil {
			return fmt.Errorf("load balancer %s not found", lbName)
		}
		lbARN = lb.ARN
	} else {
		// Interactive selector
		lbs, err := client.ListLoadBalancers()
		if err != nil {
			return fmt.Errorf("failed to list load balancers: %w", err)
		}

		selected, err := ui.SelectLoadBalancer(lbs)
		if err != nil {
			return err
		}
		lbName = selected.Name
		lbARN = selected.ARN
	}

	// Get target groups
	tgs, err := client.ListTargetGroups(lbARN)
	if err != nil {
		return fmt.Errorf("failed to list target groups: %w", err)
	}

	if len(tgs) == 0 {
		fmt.Printf("No target groups found for %s\n", lbName)
		return nil
	}

	fmt.Printf("Targets for %s:\n\n", lbName)

	// For each target group, get and display targets
	for _, tg := range tgs {
		fmt.Printf("Target Group: %s (%s:%d)\n", tg.Name, tg.Protocol, tg.Port)

		targets, err := client.ListTargets(tg.ARN)
		if err != nil {
			fmt.Printf("  Error listing targets: %v\n", err)
			continue
		}

		if len(targets) == 0 {
			fmt.Println("  No targets registered")
		} else {
			ui.PrintTargetTable(targets)
		}
		fmt.Println()
	}

	return nil
}
