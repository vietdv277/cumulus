package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

const (
	listHeight       = 8
	detailLabelWidth = 12
	minWidth         = 60
	maxWidth         = 120
	// Fixed column widths
	colWidthID    = 21
	colWidthIP    = 15
	colWidthState = 10
)

// Model represents the bubbletea model for instance selection
type Model struct {
	instances    []pkgtypes.Instance
	filtered     []pkgtypes.Instance
	cursor       int
	offset       int // for scrolling
	search       string
	selected     *pkgtypes.Instance
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int   // width inside the box (excluding borders)
	colWidths    []int // [ID, IP, State, Name]
}

// NewModel creates a new selector model
func NewModel(instances []pkgtypes.Instance) Model {
	m := Model{
		instances: instances,
		filtered:  instances,
		cursor:    0,
		offset:    0,
		search:    "",
		termWidth: 80, // default
	}
	m.calculateWidths()
	return m
}

// calculateWidths computes responsive column widths based on terminal size
func (m *Model) calculateWidths() {
	// Content width = terminal width - 2 (for box borders)
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}

	// Fixed widths: cursor(3) + ID + spacing(2) + IP + spacing(2) + State + spacing(2) + Name
	// Available for name = contentWidth - 3 - ID - 2 - IP - 2 - State - 2
	fixedWidth := 3 + colWidthID + 2 + colWidthIP + 2 + colWidthState + 2
	nameWidth := m.contentWidth - fixedWidth
	if nameWidth < 10 {
		nameWidth = 10
	}

	m.colWidths = []int{colWidthID, colWidthIP, colWidthState, nameWidth}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				if m.cursor >= m.offset+listHeight {
					m.offset = m.cursor - listHeight + 1
				}
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterInstances()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterInstances()
		}
	}

	return m, nil
}

// filterInstances filters the instances based on search query
func (m *Model) filterInstances() {
	if m.search == "" {
		m.filtered = m.instances
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, inst := range m.instances {
			if strings.Contains(strings.ToLower(inst.Name), query) ||
				strings.Contains(strings.ToLower(inst.ID), query) ||
				strings.Contains(strings.ToLower(inst.PrivateIP), query) ||
				strings.Contains(strings.ToLower(inst.ASG), query) {
				m.filtered = append(m.filtered, inst)
			}
		}
	}
	// Reset cursor if out of bounds
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
func (m Model) View() string {
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

	// Empty line after search
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Instance list
	visibleEnd := m.offset + listHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}

	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderInstanceRow(i))
	}

	// Fill remaining lines if list is short
	for i := len(m.filtered); i < m.offset+listHeight; i++ {
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

func (m Model) renderInstanceRow(idx int) string {
	var sb strings.Builder
	inst := m.filtered[idx]
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))

	// Track plain text width as we build the line
	var line strings.Builder
	plainWidth := 0

	// Cursor indicator (3 chars)
	if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	// ID column
	idText := padRight(inst.ID, m.colWidths[0])
	line.WriteString(IDStyle.Render(idText))
	line.WriteString("  ")
	plainWidth += m.colWidths[0] + 2

	// IP column
	ipText := padRight(inst.PrivateIP, m.colWidths[1])
	line.WriteString(IPStyle.Render(ipText))
	line.WriteString("  ")
	plainWidth += m.colWidths[1] + 2

	// State column
	stateText := formatStatePlain(inst.State, m.colWidths[2])
	line.WriteString(formatStateStyled(inst.State, stateText))
	line.WriteString("  ")
	plainWidth += m.colWidths[2] + 2

	// Name column
	nameText := padRight(inst.Name, m.colWidths[3])
	line.WriteString(NameStyle.Render(nameText))
	plainWidth += m.colWidths[3]

	// Pad to fill width
	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) renderDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" Instance Details", w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Underline
	sb.WriteString(BorderStyle.Render(Vertical))
	underline := " " + strings.Repeat("─", 20)
	sb.WriteString(MutedStyle.Render(padToWidth(underline, w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	if len(m.filtered) == 0 {
		// No instances
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(MutedStyle.Render(padToWidth(" No instances found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")

		// Empty lines
		for i := 0; i < 8; i++ {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	} else {
		inst := m.filtered[m.cursor]

		// Detail rows
		details := []struct {
			label string
			value string
			style lipgloss.Style
		}{
			{"ID:", inst.ID, IDStyle},
			{"Name:", inst.Name, NameStyle},
			{"Private IP:", inst.PrivateIP, IPStyle},
			{"Public IP:", formatOptional(inst.PublicIP), IPStyle},
			{"State:", inst.State, getStateStyle(inst.State)},
			{"Type:", inst.Type, TypeStyle},
			{"AZ:", inst.AZ, AZStyle},
			{"ASG:", formatOptional(inst.ASG), ASGStyle},
			{"Launch:", inst.LaunchTime.Format("2006-01-02 15:04:05"), MutedStyle},
		}

		for _, d := range details {
			sb.WriteString(BorderStyle.Render(Vertical))

			// Build the line: " Label:      Value"
			labelText := padRight(d.label, detailLabelWidth)
			valueText := d.value

			// Calculate how much value text we can show
			maxValueWidth := w - 1 - detailLabelWidth
			if runewidth.StringWidth(valueText) > maxValueWidth {
				valueText = runewidth.Truncate(valueText, maxValueWidth, "...")
			}

			// Plain width calculation
			plainWidth := 1 + detailLabelWidth + runewidth.StringWidth(valueText)

			// Build styled line
			line := MutedStyle.Render(" "+labelText) + d.style.Render(valueText)

			// Pad to fill
			if plainWidth < w {
				line += strings.Repeat(" ", w-plainWidth)
			}

			sb.WriteString(line)
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	}

	// Empty line at end
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m Model) renderStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2 // include border width for status bar

	countInfo := fmt.Sprintf("  %d/%d instances", len(m.filtered), len(m.instances))
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

// formatStatePlain returns the plain text state with indicator, padded to width
func formatStatePlain(state string, width int) string {
	var indicator string

	switch state {
	case "running":
		indicator = "●"
	case "stopped":
		indicator = "○"
	case "pending", "stopping":
		indicator = "◐"
	default:
		indicator = "○"
	}

	stateText := indicator + " " + state
	return padRight(stateText, width)
}

// formatStateStyled applies the appropriate style to the state text
func formatStateStyled(state string, text string) string {
	switch state {
	case "running":
		return RunningStyle.Render(text)
	case "stopped":
		return StoppedStyle.Render(text)
	case "pending", "stopping":
		return PendingStyle.Render(text)
	default:
		return StoppedStyle.Render(text)
	}
}

func getStateStyle(state string) lipgloss.Style {
	switch state {
	case "running":
		return RunningStyle
	case "stopped":
		return StoppedStyle
	case "pending", "stopping":
		return PendingStyle
	default:
		return StoppedStyle
	}
}

func formatOptional(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func padToWidth(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}

// SelectInstance displays an interactive selector for EC2 instances
// and returns the selected instance
func SelectInstance(instances []pkgtypes.Instance) (*pkgtypes.Instance, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances available")
	}

	m := NewModel(instances)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(Model)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
