package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// Column widths (display width, not byte length)
var columnWidths = []int{22, 26, 14, 11, 12, 18, 20}

// PrintInstanceTable prints instances in a styled box table
func PrintInstanceTable(instances []pkgtypes.Instance) {
	headers := []string{"ID", "Name", "Private IP", "State", "Type", "AZ", "ASG"}

	// Build table
	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range columnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, columnWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range columnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, inst := range instances {
		sb.WriteString(BorderStyle.Render(Vertical))

		// ID
		cell := " " + padRight(inst.ID, columnWidths[0]) + " "
		sb.WriteString(IDStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell = " " + padRight(inst.Name, columnWidths[1]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Private IP
		cell = " " + padRight(inst.PrivateIP, columnWidths[2]) + " "
		sb.WriteString(IPStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// State with indicator
		stateCell := formatState(inst.State, columnWidths[3])
		sb.WriteString(stateCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		// Type
		cell = " " + padRight(inst.Type, columnWidths[4]) + " "
		sb.WriteString(TypeStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// AZ
		cell = " " + padRight(inst.AZ, columnWidths[5]) + " "
		sb.WriteString(AZStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// ASG
		cell = " " + padRight(inst.ASG, columnWidths[6]) + " "
		sb.WriteString(ASGStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range columnWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	// Print the table
	fmt.Print(sb.String())

	// Summary
	printSummary(instances)
}

func formatState(state string, width int) string {
	var indicator string
	var style = StoppedStyle

	switch state {
	case "running":
		indicator = "●"
		style = RunningStyle
	case "stopped":
		indicator = "○"
		style = StoppedStyle
	case "pending", "stopping":
		indicator = "◐"
		style = PendingStyle
	default:
		indicator = "○"
		style = StoppedStyle
	}

	// Format: " ● state " with proper padding
	stateText := indicator + " " + state
	stateWidth := runewidth.StringWidth(stateText)

	// Pad to fill the column width
	padding := width - stateWidth
	if padding < 0 {
		padding = 0
	}

	cell := " " + stateText + strings.Repeat(" ", padding) + " "
	return style.Render(cell)
}

func printSummary(instances []pkgtypes.Instance) {
	counts := make(map[string]int)
	for _, inst := range instances {
		counts[inst.State]++
	}

	var parts []string
	if c := counts["running"]; c > 0 {
		parts = append(parts, RunningStyle.Render(fmt.Sprintf("%d running", c)))
	}
	if c := counts["stopped"]; c > 0 {
		parts = append(parts, StoppedStyle.Render(fmt.Sprintf("%d stopped", c)))
	}
	if c := counts["pending"]; c > 0 {
		parts = append(parts, PendingStyle.Render(fmt.Sprintf("%d pending", c)))
	}
	if c := counts["stopping"]; c > 0 {
		parts = append(parts, PendingStyle.Render(fmt.Sprintf("%d stopping", c)))
	}

	total := len(instances)
	summary := fmt.Sprintf("  %d instances", total)
	if len(parts) > 0 {
		summary += " (" + strings.Join(parts, ", ") + ")"
	}

	fmt.Println(summary)
}
