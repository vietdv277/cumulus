package ui

import (
	"fmt"
	"strings"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// LB table column widths
var lbColumnWidths = []int{30, 14, 16, 12, 50}

// Target table column widths
var targetColumnWidths = []int{22, 8, 16, 12}

// PrintLBTable prints load balancers in a styled box table
func PrintLBTable(lbs []pkgtypes.LoadBalancer) {
	headers := []string{"Name", "Type", "Scheme", "State", "DNS Name"}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range lbColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(lbColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, lbColumnWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range lbColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(lbColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, lb := range lbs {
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell := " " + padRight(lb.Name, lbColumnWidths[0]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Type
		cell = " " + padRight(lb.Type, lbColumnWidths[1]) + " "
		sb.WriteString(TypeStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Scheme
		cell = " " + padRight(lb.Scheme, lbColumnWidths[2]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// State
		stateCell := formatLBState(lb.State, lbColumnWidths[3])
		sb.WriteString(stateCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		// DNS Name
		cell = " " + padRight(lb.DNSName, lbColumnWidths[4]) + " "
		sb.WriteString(IPStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range lbColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(lbColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d load balancers\n", len(lbs))
}

// PrintTargetTable prints targets in a styled box table
func PrintTargetTable(targets []pkgtypes.Target) {
	headers := []string{"Target ID", "Port", "AZ", "Health"}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range targetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(targetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, targetColumnWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range targetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(targetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, target := range targets {
		sb.WriteString(BorderStyle.Render(Vertical))

		// Target ID
		cell := " " + padRight(target.ID, targetColumnWidths[0]) + " "
		sb.WriteString(IDStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Port
		portStr := fmt.Sprintf("%d", target.Port)
		cell = " " + padRight(portStr, targetColumnWidths[1]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// AZ
		cell = " " + padRight(target.AZ, targetColumnWidths[2]) + " "
		sb.WriteString(AZStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Health
		healthCell := formatHealthState(target.Health, targetColumnWidths[3])
		sb.WriteString(healthCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range targetColumnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(targetColumnWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())

	// Summary
	printTargetSummary(targets)
}

// PrintTargetGroupTable prints target groups in a styled box table
func PrintTargetGroupTable(tgs []pkgtypes.TargetGroup) {
	headers := []string{"Name", "Protocol", "Port", "Type"}
	widths := []int{40, 10, 8, 12}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range widths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, widths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range widths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, tg := range tgs {
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell := " " + padRight(tg.Name, widths[0]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Protocol
		cell = " " + padRight(tg.Protocol, widths[1]) + " "
		sb.WriteString(TypeStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Port
		portStr := fmt.Sprintf("%d", tg.Port)
		cell = " " + padRight(portStr, widths[2]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Type
		cell = " " + padRight(tg.Type, widths[3]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range widths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d target groups\n", len(tgs))
}

func formatLBState(state string, width int) string {
	var indicator string
	var style = MutedStyle

	switch state {
	case "active":
		indicator = "●"
		style = RunningStyle
	case "provisioning":
		indicator = "◐"
		style = PendingStyle
	case "active_impaired":
		indicator = "◐"
		style = PendingStyle
	case "failed":
		indicator = "○"
		style = StoppedStyle
	default:
		indicator = "○"
		style = MutedStyle
	}

	stateText := indicator + " " + state
	cell := " " + padRight(stateText, width) + " "
	return style.Render(cell)
}

func formatHealthState(health string, width int) string {
	var indicator string
	var style = MutedStyle

	switch health {
	case "healthy":
		indicator = "●"
		style = RunningStyle
	case "unhealthy":
		indicator = "○"
		style = StoppedStyle
	case "draining":
		indicator = "◐"
		style = PendingStyle
	case "initial":
		indicator = "◐"
		style = PendingStyle
	case "unused":
		indicator = "○"
		style = MutedStyle
	default:
		indicator = "○"
		style = MutedStyle
	}

	healthText := indicator + " " + health
	cell := " " + padRight(healthText, width) + " "
	return style.Render(cell)
}

func printTargetSummary(targets []pkgtypes.Target) {
	counts := make(map[string]int)
	for _, t := range targets {
		counts[t.Health]++
	}

	var parts []string
	if c := counts["healthy"]; c > 0 {
		parts = append(parts, RunningStyle.Render(fmt.Sprintf("%d healthy", c)))
	}
	if c := counts["unhealthy"]; c > 0 {
		parts = append(parts, StoppedStyle.Render(fmt.Sprintf("%d unhealthy", c)))
	}
	if c := counts["draining"]; c > 0 {
		parts = append(parts, PendingStyle.Render(fmt.Sprintf("%d draining", c)))
	}
	if c := counts["initial"]; c > 0 {
		parts = append(parts, PendingStyle.Render(fmt.Sprintf("%d initial", c)))
	}
	if c := counts["unused"]; c > 0 {
		parts = append(parts, MutedStyle.Render(fmt.Sprintf("%d unused", c)))
	}

	total := len(targets)
	summary := fmt.Sprintf("  %d targets", total)
	if len(parts) > 0 {
		summary += " (" + strings.Join(parts, ", ") + ")"
	}

	fmt.Println(summary)
}
