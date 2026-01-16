package ui

import (
	"fmt"
	"strings"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// VPC table column widths
var vpcColumnWidths = []int{24, 30, 18, 12, 10}

// Subnet table column widths
var subnetColumnWidths = []int{26, 30, 18, 14, 8, 10, 8}

// PrintVPCTable prints VPCs in a styled box table
func PrintVPCTable(vpcs []pkgtypes.VPC) {
	headers := []string{"ID", "Name", "CIDR", "State", "Default"}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range vpcColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(vpcColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, vpcColumnWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range vpcColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(vpcColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, vpc := range vpcs {
		sb.WriteString(BorderStyle.Render(Vertical))

		// ID
		cell := " " + padRight(vpc.ID, vpcColumnWidths[0]) + " "
		sb.WriteString(IDStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell = " " + padRight(vpc.Name, vpcColumnWidths[1]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// CIDR
		cell = " " + padRight(vpc.CIDR, vpcColumnWidths[2]) + " "
		sb.WriteString(IPStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// State
		stateCell := formatVPCState(vpc.State, vpcColumnWidths[3])
		sb.WriteString(stateCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		// Default
		defaultStr := "No"
		if vpc.IsDefault {
			defaultStr = "Yes"
		}
		cell = " " + padRight(defaultStr, vpcColumnWidths[4]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range vpcColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(vpcColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d VPCs\n", len(vpcs))
}

// PrintSubnetTable prints subnets in a styled box table
func PrintSubnetTable(subnets []pkgtypes.Subnet) {
	headers := []string{"ID", "Name", "CIDR", "AZ", "IPs", "State", "Public"}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range subnetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(subnetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, subnetColumnWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range subnetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(subnetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, subnet := range subnets {
		sb.WriteString(BorderStyle.Render(Vertical))

		// ID
		cell := " " + padRight(subnet.ID, subnetColumnWidths[0]) + " "
		sb.WriteString(IDStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell = " " + padRight(subnet.Name, subnetColumnWidths[1]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// CIDR
		cell = " " + padRight(subnet.CIDR, subnetColumnWidths[2]) + " "
		sb.WriteString(IPStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// AZ
		cell = " " + padRight(subnet.AZ, subnetColumnWidths[3]) + " "
		sb.WriteString(AZStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Available IPs
		ipsStr := fmt.Sprintf("%d", subnet.AvailableIPs)
		cell = " " + padRight(ipsStr, subnetColumnWidths[4]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// State
		stateCell := formatSubnetState(subnet.State, subnetColumnWidths[5])
		sb.WriteString(stateCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		// Public
		publicStr := "No"
		if subnet.Public {
			publicStr = "Yes"
		}
		cell = " " + padRight(publicStr, subnetColumnWidths[6]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range subnetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(subnetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d subnets\n", len(subnets))
}

func formatVPCState(state string, width int) string {
	var indicator string
	var style = MutedStyle

	switch state {
	case "available":
		indicator = "●"
		style = RunningStyle
	case "pending":
		indicator = "◐"
		style = PendingStyle
	default:
		indicator = "○"
		style = StoppedStyle
	}

	stateText := indicator + " " + state
	cell := " " + padRight(stateText, width) + " "
	return style.Render(cell)
}

func formatSubnetState(state string, width int) string {
	var indicator string
	var style = MutedStyle

	switch state {
	case "available":
		indicator = "●"
		style = RunningStyle
	case "pending":
		indicator = "◐"
		style = PendingStyle
	default:
		indicator = "○"
		style = StoppedStyle
	}

	stateText := indicator + " " + state
	cell := " " + padRight(stateText, width) + " "
	return style.Render(cell)
}
