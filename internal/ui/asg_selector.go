package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

const (
	asgListHeight    = 8
	asgDetailWidth   = 18 // fits "Desired/Min/Max:" (16 chars + padding)
	asgColWidthCap   = 12 // "X/Y/Z" format for desired/min/max
	asgColWidthCount = 10
)

// ASGModel represents the bubbletea model for ASG selection
type ASGModel struct {
	groups       []pkgtypes.AutoScalingGroup
	filtered     []pkgtypes.AutoScalingGroup
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.AutoScalingGroup
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
	maxNameWidth int // dynamic width for ASG names
}

// NewASGModel creates a new ASG selector model
func NewASGModel(groups []pkgtypes.AutoScalingGroup) ASGModel {
	// Calculate max name width from all groups
	maxNameWidth := 30 // minimum
	for _, g := range groups {
		nameWidth := runewidth.StringWidth(g.Name)
		if nameWidth > maxNameWidth {
			maxNameWidth = nameWidth
		}
	}

	m := ASGModel{
		groups:       groups,
		filtered:     groups,
		cursor:       0,
		offset:       0,
		search:       "",
		termWidth:    80,
		maxNameWidth: maxNameWidth,
	}
	m.calculateWidths()
	return m
}

func (m *ASGModel) calculateWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}

	// Ensure content width is large enough to fit the longest name + other columns
	// cursor(3) + name + spacing(2) + cap(12) + spacing(2) + count(10) = name + 29
	minRequiredWidth := m.maxNameWidth + 29
	if m.contentWidth < minRequiredWidth {
		m.contentWidth = minRequiredWidth
	}
}

// Init implements tea.Model
func (m ASGModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m ASGModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.calculateWidths()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			m.cancelled = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.selected = &m.filtered[m.cursor]
				m.quitting = true
				return m, tea.Quit
			}

		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}

		case tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				if m.cursor >= m.offset+asgListHeight {
					m.offset = m.cursor - asgListHeight + 1
				}
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterGroups()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterGroups()
		}
	}

	return m, nil
}

func (m *ASGModel) filterGroups() {
	if m.search == "" {
		m.filtered = m.groups
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, g := range m.groups {
			if strings.Contains(strings.ToLower(g.Name), query) {
				m.filtered = append(m.filtered, g)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
	m.offset = 0
}

// View implements tea.Model
func (m ASGModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder
	w := m.contentWidth

	// Top border
	sb.WriteString(BorderStyle.Render(TopLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	// Search input
	searchLine := " > " + m.search
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(NameStyle.Render(padToWidth(searchLine, w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Empty line
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// ASG list
	visibleEnd := m.offset + asgListHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}

	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderASGRow(i))
	}

	// Fill remaining lines
	for i := len(m.filtered); i < m.offset+asgListHeight; i++ {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(strings.Repeat(" ", w))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	// Empty line before details
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Separator
	sb.WriteString(BorderStyle.Render(LeftT))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	// Details panel
	sb.WriteString(m.renderDetailsPanel())

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	// Status bar
	sb.WriteString(m.renderStatusBar())

	return sb.String()
}

func (m ASGModel) renderASGRow(idx int) string {
	var sb strings.Builder
	asg := m.filtered[idx]
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	// Cursor
	if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	// Name (using dynamic width)
	nameText := padRight(asg.Name, m.maxNameWidth)
	line.WriteString(NameStyle.Render(nameText))
	line.WriteString("  ")
	plainWidth += m.maxNameWidth + 2

	// Capacity: desired/min/max
	capText := fmt.Sprintf("%d/%d/%d", asg.DesiredCapacity, asg.MinSize, asg.MaxSize)
	capText = padRight(capText, asgColWidthCap)
	line.WriteString(TypeStyle.Render(capText))
	line.WriteString("  ")
	plainWidth += asgColWidthCap + 2

	// Instance count
	countText := fmt.Sprintf("%d running", asg.InstanceCount)
	countText = padRight(countText, asgColWidthCount)
	line.WriteString(IPStyle.Render(countText))
	plainWidth += asgColWidthCount

	// Pad to fill
	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m ASGModel) renderDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" ASG Details", w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Underline
	sb.WriteString(BorderStyle.Render(Vertical))
	underline := " " + strings.Repeat("─", 20)
	sb.WriteString(MutedStyle.Render(padToWidth(underline, w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	if len(m.filtered) == 0 {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(MutedStyle.Render(padToWidth(" No ASGs found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")

		for i := 0; i < 6; i++ {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	} else {
		asg := m.filtered[m.cursor]

		details := []struct {
			label string
			value string
		}{
			{"Name:", asg.Name},
			{"Launch Template:", formatOptional(asg.LaunchTemplate)},
			{"Desired/Min/Max:", fmt.Sprintf("%d / %d / %d", asg.DesiredCapacity, asg.MinSize, asg.MaxSize)},
			{"Running:", fmt.Sprintf("%d instances", asg.InstanceCount)},
			{"Healthy:", fmt.Sprintf("%d / %d", asg.HealthyCount, asg.InstanceCount)},
			{"Status:", asg.Status},
			{"AZs:", strings.Join(asg.AZs, ", ")},
		}

		for _, d := range details {
			sb.WriteString(BorderStyle.Render(Vertical))

			labelText := padRight(d.label, asgDetailWidth)
			valueText := d.value

			// Don't truncate - show full value
			plainWidth := 1 + asgDetailWidth + runewidth.StringWidth(valueText)
			line := MutedStyle.Render(" "+labelText) + NameStyle.Render(valueText)

			if plainWidth < w {
				line += strings.Repeat(" ", w-plainWidth)
			}

			sb.WriteString(line)
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	}

	// Empty line
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m ASGModel) renderStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d ASGs", len(m.filtered), len(m.groups))
	hintsPlain := "[Enter:select] [Esc:cancel]"

	countWidth := runewidth.StringWidth(countInfo)
	hintsWidth := runewidth.StringWidth(hintsPlain)
	padding := w - countWidth - hintsWidth

	sb.WriteString(countInfo)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(HintStyle.Render(hintsPlain))
	sb.WriteString("\n")

	return sb.String()
}

// SelectASG displays an interactive selector for Auto Scaling Groups
func SelectASG(groups []pkgtypes.AutoScalingGroup) (*pkgtypes.AutoScalingGroup, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("no ASGs available")
	}

	m := NewASGModel(groups)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(ASGModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
