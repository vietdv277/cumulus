package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// PrintASGTable prints Auto Scaling Groups in a styled box table
func PrintASGTable(groups []pkgtypes.AutoScalingGroup) {
	headers := []string{"Name", "Desired", "Min", "Max", "Running", "Healthy", "Status"}

	// Calculate dynamic name column width
	nameWidth := len(headers[0]) // minimum is header width
	for _, g := range groups {
		w := runewidth.StringWidth(g.Name)
		if w > nameWidth {
			nameWidth = w
		}
	}

	// Column widths: Name (dynamic), Desired, Min, Max, Running, Healthy, Status
	colWidths := []int{nameWidth, 8, 6, 6, 8, 8, 12}

	var sb strings.Builder

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	for i, w := range colWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(colWidths)-1 {
			sb.WriteString(BorderStyle.Render(TopT))
		}
	}
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(BorderStyle.Render(Vertical))
	for i, h := range headers {
		cell := " " + padRight(h, colWidths[i]) + " "
		sb.WriteString(HeaderStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(BorderStyle.Render(LeftT))
	for i, w := range colWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(colWidths)-1 {
			sb.WriteString(BorderStyle.Render(Cross))
		}
	}
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Data rows
	for _, asg := range groups {
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell := " " + padRight(asg.Name, colWidths[0]) + " "
		sb.WriteString(NameStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Desired
		cell = " " + padRight(fmt.Sprintf("%d", asg.DesiredCapacity), colWidths[1]) + " "
		sb.WriteString(TypeStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Min
		cell = " " + padRight(fmt.Sprintf("%d", asg.MinSize), colWidths[2]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Max
		cell = " " + padRight(fmt.Sprintf("%d", asg.MaxSize), colWidths[3]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Running (instance count)
		cell = " " + padRight(fmt.Sprintf("%d", asg.InstanceCount), colWidths[4]) + " "
		sb.WriteString(IPStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Healthy
		healthCell := formatHealthCount(asg.HealthyCount, asg.InstanceCount, colWidths[5])
		sb.WriteString(healthCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		// Status
		statusCell := formatASGStatus(asg.Status, colWidths[6])
		sb.WriteString(statusCell)
		sb.WriteString(BorderStyle.Render(Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	for i, w := range colWidths {
		sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w+2)))
		if i < len(colWidths)-1 {
			sb.WriteString(BorderStyle.Render(BottomT))
		}
	}
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())

	// Summary
	fmt.Printf("  %d Auto Scaling Groups\n", len(groups))
}

func formatHealthCount(healthy, total int, width int) string {
	text := fmt.Sprintf("%d/%d", healthy, total)
	cell := " " + padRight(text, width) + " "

	if healthy == total && total > 0 {
		return RunningStyle.Render(cell)
	} else if healthy == 0 && total > 0 {
		return StoppedStyle.Render(cell)
	}
	return PendingStyle.Render(cell)
}

func formatASGStatus(status string, width int) string {
	cell := " " + padRight(status, width) + " "

	switch status {
	case "InService", "":
		return RunningStyle.Render(cell)
	case "Updating", "Pending":
		return PendingStyle.Render(cell)
	default:
		return MutedStyle.Render(cell)
	}
}

// PrintASGDetails prints detailed information about an ASG
func PrintASGDetails(asg *pkgtypes.AutoScalingGroup) {
	var sb strings.Builder
	labelWidth := 20

	// Details
	details := []struct {
		label string
		value string
	}{
		{"Name:", asg.Name},
		{"Launch Template:", formatOptional(asg.LaunchTemplate)},
		{"Desired Capacity:", fmt.Sprintf("%d", asg.DesiredCapacity)},
		{"Min Size:", fmt.Sprintf("%d", asg.MinSize)},
		{"Max Size:", fmt.Sprintf("%d", asg.MaxSize)},
		{"Running Instances:", fmt.Sprintf("%d", asg.InstanceCount)},
		{"Healthy:", fmt.Sprintf("%d/%d", asg.HealthyCount, asg.InstanceCount)},
		{"Status:", asg.Status},
		{"Availability Zones:", strings.Join(asg.AZs, ", ")},
		{"Created:", asg.CreatedTime.Format("2006-01-02 15:04:05")},
	}

	// Calculate width based on longest value (using display width)
	minWidth := 60
	width := minWidth
	for _, d := range details {
		valueWidth := runewidth.StringWidth(d.value)
		lineLen := 1 + labelWidth + valueWidth + 1 // " " + label + value + " "
		if lineLen > width {
			width = lineLen
		}
	}

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, width)))
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Title
	sb.WriteString(BorderStyle.Render(Vertical))
	title := " Auto Scaling Group Details"
	sb.WriteString(HeaderStyle.Render(padRight(title, width)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Separator
	sb.WriteString(BorderStyle.Render(LeftT))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, width)))
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	for _, d := range details {
		sb.WriteString(BorderStyle.Render(Vertical))
		// Build line without truncation
		line := " " + padRight(d.label, labelWidth) + d.value
		lineWidth := runewidth.StringWidth(line)
		// Pad to fill remaining width
		if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}
		sb.WriteString(line)
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, width)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())

	// Print instances if any
	if len(asg.Instances) > 0 {
		fmt.Println()
		fmt.Println("  Instances:")
		PrintInstanceTable(asg.Instances)
	}
}
