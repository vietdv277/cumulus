package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// Box drawing characters
const (
	topLeft     = "╭"
	topRight    = "╮"
	bottomLeft  = "╰"
	bottomRight = "╯"
	horizontal  = "─"
	vertical    = "│"
	leftT       = "├"
	rightT      = "┤"
	topT        = "┬"
	bottomT     = "┴"
	cross       = "┼"
)

// Column widths
var columnWidths = []int{22, 26, 14, 11, 12, 18, 20}

// Styles
var (
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	idStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	nameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	ipStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	typeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	azStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	asgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	stoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// PrintInstanceTable prints instances in a styled box table
func PrintInstanceTable(instances []pkgtypes.Instance) {
	headers := []string{"ID", "Name", "Private IP", "State", "Type", "AZ", "ASG"}

	// Build table
	var sb strings.Builder

	// Top border
	sb.WriteString(borderStyle.Render(topLeft))
	for i, w := range columnWidths {
		sb.WriteString(borderStyle.Render(strings.Repeat(horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(borderStyle.Render(topT))
		}
	}
	sb.WriteString(borderStyle.Render(topRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(borderStyle.Render(vertical))
	for i, h := range headers {
		cell := fmt.Sprintf(" %-*s ", columnWidths[i], truncateStr(h, columnWidths[i]))
		sb.WriteString(headerStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(borderStyle.Render(leftT))
	for i, w := range columnWidths {
		sb.WriteString(borderStyle.Render(strings.Repeat(horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(borderStyle.Render(cross))
		}
	}
	sb.WriteString(borderStyle.Render(rightT))
	sb.WriteString("\n")

	// Data rows
	for _, inst := range instances {
		sb.WriteString(borderStyle.Render(vertical))

		// ID
		cell := fmt.Sprintf(" %-*s ", columnWidths[0], truncateStr(inst.ID, columnWidths[0]))
		sb.WriteString(idStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		// Name
		cell = fmt.Sprintf(" %-*s ", columnWidths[1], truncateStr(inst.Name, columnWidths[1]))
		sb.WriteString(nameStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		// Private IP
		cell = fmt.Sprintf(" %-*s ", columnWidths[2], truncateStr(inst.PrivateIP, columnWidths[2]))
		sb.WriteString(ipStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		// State with indicator
		stateCell := formatState(inst.State, columnWidths[3])
		sb.WriteString(stateCell)
		sb.WriteString(borderStyle.Render(vertical))

		// Type
		cell = fmt.Sprintf(" %-*s ", columnWidths[4], truncateStr(inst.Type, columnWidths[4]))
		sb.WriteString(typeStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		// AZ
		cell = fmt.Sprintf(" %-*s ", columnWidths[5], truncateStr(inst.AZ, columnWidths[5]))
		sb.WriteString(azStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		// ASG
		cell = fmt.Sprintf(" %-*s ", columnWidths[6], truncateStr(inst.ASG, columnWidths[6]))
		sb.WriteString(asgStyle.Render(cell))
		sb.WriteString(borderStyle.Render(vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(borderStyle.Render(bottomLeft))
	for i, w := range columnWidths {
		sb.WriteString(borderStyle.Render(strings.Repeat(horizontal, w+2)))
		if i < len(columnWidths)-1 {
			sb.WriteString(borderStyle.Render(bottomT))
		}
	}
	sb.WriteString(borderStyle.Render(bottomRight))
	sb.WriteString("\n")

	// Print the table
	fmt.Print(sb.String())

	// Summary
	printSummary(instances)
}

func formatState(state string, width int) string {
	var indicator string
	var style lipgloss.Style

	switch state {
	case "running":
		indicator = "●"
		style = runningStyle
	case "stopped":
		indicator = "○"
		style = stoppedStyle
	case "pending", "stopping":
		indicator = "◐"
		style = pendingStyle
	default:
		indicator = "○"
		style = stoppedStyle
	}

	text := fmt.Sprintf(" %s %-*s ", indicator, width-3, state)
	return style.Render(text)
}

func printSummary(instances []pkgtypes.Instance) {
	counts := make(map[string]int)
	for _, inst := range instances {
		counts[inst.State]++
	}

	var parts []string
	if c := counts["running"]; c > 0 {
		parts = append(parts, runningStyle.Render(fmt.Sprintf("%d running", c)))
	}
	if c := counts["stopped"]; c > 0 {
		parts = append(parts, stoppedStyle.Render(fmt.Sprintf("%d stopped", c)))
	}
	if c := counts["pending"]; c > 0 {
		parts = append(parts, pendingStyle.Render(fmt.Sprintf("%d pending", c)))
	}
	if c := counts["stopping"]; c > 0 {
		parts = append(parts, pendingStyle.Render(fmt.Sprintf("%d stopping", c)))
	}

	total := len(instances)
	summary := fmt.Sprintf("  %d instances", total)
	if len(parts) > 0 {
		summary += " (" + strings.Join(parts, ", ") + ")"
	}

	fmt.Println(summary)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
