package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/ui"
)

var vpcCmd = &cobra.Command{
	Use:   "vpc",
	Short: "Manage VPCs",
	Long:  `Perform various operations on VPCs such as listing and describing VPCs and their subnets.`,
}

var vpcLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all VPCs",
	Long: `List all VPCs with their CIDR, state, name, and default flag.

Examples:
  cml vpc ls              # List all VPCs
  cml vpc ls -p prod      # List VPCs using production profile`,
	RunE: runVPCList,
}

var vpcDescribeCmd = &cobra.Command{
	Use:   "describe [vpc-id]",
	Short: "Show detailed VPC information",
	Long: `Show detailed information about a VPC including its subnets.
If no VPC ID is provided, an interactive selector will be shown.

Examples:
  cml vpc describe                  # Interactive VPC selector
  cml vpc describe vpc-12345678     # Describe specific VPC`,
	RunE: runVPCDescribe,
}

var vpcSubnetsCmd = &cobra.Command{
	Use:   "subnets [vpc-id]",
	Short: "List subnets in a VPC",
	Long: `List all subnets in a VPC with their CIDR, AZ, and availability.
If no VPC ID is provided, an interactive selector will be shown.

Examples:
  cml vpc subnets                   # Interactive VPC selector
  cml vpc subnets vpc-12345678      # List subnets in specific VPC`,
	RunE: runVPCSubnets,
}

func init() {
	rootCmd.AddCommand(vpcCmd)

	vpcCmd.AddCommand(vpcLsCmd)
	vpcCmd.AddCommand(vpcDescribeCmd)
	vpcCmd.AddCommand(vpcSubnetsCmd)
}

func runVPCList(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	vpcs, err := client.ListVPCs()
	if err != nil {
		return fmt.Errorf("failed to list VPCs: %w", err)
	}

	if len(vpcs) == 0 {
		fmt.Println("No VPCs found")
		return nil
	}

	ui.PrintVPCTable(vpcs)
	return nil
}

func runVPCDescribe(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var vpcID string

	if len(args) > 0 {
		vpcID = args[0]
	} else {
		// Interactive selector
		vpcs, err := client.ListVPCs()
		if err != nil {
			return fmt.Errorf("failed to list VPCs: %w", err)
		}

		selected, err := ui.SelectVPC(vpcs)
		if err != nil {
			return err
		}
		vpcID = selected.ID
	}

	// Get VPC details
	vpc, err := client.DescribeVPC(vpcID)
	if err != nil {
		return fmt.Errorf("failed to describe VPC: %w", err)
	}

	if vpc == nil {
		return fmt.Errorf("VPC %s not found", vpcID)
	}

	// Print VPC details
	fmt.Println()
	fmt.Printf("VPC: %s\n", vpc.ID)
	fmt.Printf("  Name:     %s\n", vpc.Name)
	fmt.Printf("  CIDR:     %s\n", vpc.CIDR)
	fmt.Printf("  State:    %s\n", vpc.State)
	fmt.Printf("  Default:  %v\n", vpc.IsDefault)
	fmt.Printf("  Owner:    %s\n", vpc.OwnerID)
	fmt.Println()

	// Get and print subnets
	subnets, err := client.ListSubnets(vpcID)
	if err != nil {
		return fmt.Errorf("failed to list subnets: %w", err)
	}

	if len(subnets) > 0 {
		fmt.Println("Subnets:")
		ui.PrintSubnetTable(subnets)
	} else {
		fmt.Println("No subnets found in this VPC")
	}

	return nil
}

func runVPCSubnets(cmd *cobra.Command, args []string) error {
	client, err := aws.NewClient(
		context.Background(),
		aws.WithProfile(GetProfile()),
		aws.WithRegion(GetRegion()),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	var vpcID string

	if len(args) > 0 {
		vpcID = args[0]
	} else {
		// Interactive selector
		vpcs, err := client.ListVPCs()
		if err != nil {
			return fmt.Errorf("failed to list VPCs: %w", err)
		}

		selected, err := ui.SelectVPC(vpcs)
		if err != nil {
			return err
		}
		vpcID = selected.ID
	}

	// Get subnets
	subnets, err := client.ListSubnets(vpcID)
	if err != nil {
		return fmt.Errorf("failed to list subnets: %w", err)
	}

	if len(subnets) == 0 {
		fmt.Println("No subnets found in this VPC")
		return nil
	}

	ui.PrintSubnetTable(subnets)
	return nil
}
