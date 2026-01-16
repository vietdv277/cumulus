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
	vpcListHeight       = 8
	vpcDetailLabelWidth = 12
)

// VPCModel represents the bubbletea model for VPC selection
type VPCModel struct {
	vpcs         []pkgtypes.VPC
	filtered     []pkgtypes.VPC
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.VPC
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
}

// NewVPCModel creates a new VPC selector model
func NewVPCModel(vpcs []pkgtypes.VPC) VPCModel {
	m := VPCModel{
		vpcs:      vpcs,
		filtered:  vpcs,
		cursor:    0,
		offset:    0,
		search:    "",
		termWidth: 80,
	}
	m.calculateWidths()
	return m
}

func (m *VPCModel) calculateWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}
}

// Init implements tea.Model
func (m VPCModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m VPCModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				if m.cursor >= m.offset+vpcListHeight {
					m.offset = m.cursor - vpcListHeight + 1
				}
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterVPCs()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterVPCs()
		}
	}

	return m, nil
}

func (m *VPCModel) filterVPCs() {
	if m.search == "" {
		m.filtered = m.vpcs
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, vpc := range m.vpcs {
			if strings.Contains(strings.ToLower(vpc.Name), query) ||
				strings.Contains(strings.ToLower(vpc.ID), query) ||
				strings.Contains(strings.ToLower(vpc.CIDR), query) {
				m.filtered = append(m.filtered, vpc)
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
func (m VPCModel) View() string {
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

	// VPC list
	visibleEnd := m.offset + vpcListHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}

	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderVPCRow(i))
	}

	// Fill remaining lines
	for i := len(m.filtered); i < m.offset+vpcListHeight; i++ {
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

func (m VPCModel) renderVPCRow(idx int) string {
	var sb strings.Builder
	vpc := m.filtered[idx]
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	// Cursor indicator
	if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	// ID
	idText := padRight(vpc.ID, 24)
	line.WriteString(IDStyle.Render(idText))
	line.WriteString("  ")
	plainWidth += 24 + 2

	// CIDR
	cidrText := padRight(vpc.CIDR, 18)
	line.WriteString(IPStyle.Render(cidrText))
	line.WriteString("  ")
	plainWidth += 18 + 2

	// Name
	nameWidth := w - plainWidth
	if nameWidth < 10 {
		nameWidth = 10
	}
	nameText := padRight(vpc.Name, nameWidth)
	line.WriteString(NameStyle.Render(nameText))
	plainWidth += nameWidth

	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m VPCModel) renderDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" VPC Details", w)))
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
		sb.WriteString(MutedStyle.Render(padToWidth(" No VPCs found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")

		for i := 0; i < 4; i++ {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	} else {
		vpc := m.filtered[m.cursor]

		details := []struct {
			label string
			value string
			style lipgloss.Style
		}{
			{"ID:", vpc.ID, IDStyle},
			{"Name:", vpc.Name, NameStyle},
			{"CIDR:", vpc.CIDR, IPStyle},
			{"State:", vpc.State, RunningStyle},
			{"Default:", formatBool(vpc.IsDefault), MutedStyle},
			{"Owner:", vpc.OwnerID, MutedStyle},
		}

		for _, d := range details {
			sb.WriteString(BorderStyle.Render(Vertical))

			labelText := padRight(d.label, vpcDetailLabelWidth)
			valueText := d.value

			maxValueWidth := w - 1 - vpcDetailLabelWidth
			if runewidth.StringWidth(valueText) > maxValueWidth {
				valueText = runewidth.Truncate(valueText, maxValueWidth, "...")
			}

			plainWidth := 1 + vpcDetailLabelWidth + runewidth.StringWidth(valueText)
			line := MutedStyle.Render(" "+labelText) + d.style.Render(valueText)

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

func (m VPCModel) renderStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d VPCs", len(m.filtered), len(m.vpcs))
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

func formatBool(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// SelectVPC displays an interactive selector for VPCs
func SelectVPC(vpcs []pkgtypes.VPC) (*pkgtypes.VPC, error) {
	if len(vpcs) == 0 {
		return nil, fmt.Errorf("no VPCs available")
	}

	m := NewVPCModel(vpcs)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(VPCModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
