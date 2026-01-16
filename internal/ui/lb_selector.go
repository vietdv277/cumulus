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
	lbListHeight       = 8
	lbDetailLabelWidth = 12
)

// LBModel represents the bubbletea model for Load Balancer selection
type LBModel struct {
	lbs          []pkgtypes.LoadBalancer
	filtered     []pkgtypes.LoadBalancer
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.LoadBalancer
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
}

// NewLBModel creates a new LB selector model
func NewLBModel(lbs []pkgtypes.LoadBalancer) LBModel {
	m := LBModel{
		lbs:       lbs,
		filtered:  lbs,
		cursor:    0,
		offset:    0,
		search:    "",
		termWidth: 80,
	}
	m.calculateWidths()
	return m
}

func (m *LBModel) calculateWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}
}

// Init implements tea.Model
func (m LBModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m LBModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				if m.cursor >= m.offset+lbListHeight {
					m.offset = m.cursor - lbListHeight + 1
				}
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterLBs()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterLBs()
		}
	}

	return m, nil
}

func (m *LBModel) filterLBs() {
	if m.search == "" {
		m.filtered = m.lbs
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, lb := range m.lbs {
			if strings.Contains(strings.ToLower(lb.Name), query) ||
				strings.Contains(strings.ToLower(lb.DNSName), query) ||
				strings.Contains(strings.ToLower(lb.Type), query) {
				m.filtered = append(m.filtered, lb)
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
func (m LBModel) View() string {
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

	// LB list
	visibleEnd := m.offset + lbListHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}

	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderLBRow(i))
	}

	// Fill remaining lines
	for i := len(m.filtered); i < m.offset+lbListHeight; i++ {
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

func (m LBModel) renderLBRow(idx int) string {
	var sb strings.Builder
	lb := m.filtered[idx]
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

	// Name
	nameText := padRight(lb.Name, 30)
	line.WriteString(NameStyle.Render(nameText))
	line.WriteString("  ")
	plainWidth += 30 + 2

	// Type
	typeText := padRight(lb.Type, 12)
	line.WriteString(TypeStyle.Render(typeText))
	line.WriteString("  ")
	plainWidth += 12 + 2

	// State
	stateText := padRight(lb.State, 10)
	line.WriteString(formatLBStateInline(lb.State, stateText))
	plainWidth += 10

	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func formatLBStateInline(state, text string) string {
	switch state {
	case "active":
		return RunningStyle.Render(text)
	case "provisioning", "active_impaired":
		return PendingStyle.Render(text)
	case "failed":
		return StoppedStyle.Render(text)
	default:
		return MutedStyle.Render(text)
	}
}

func (m LBModel) renderDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" Load Balancer Details", w)))
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
		sb.WriteString(MutedStyle.Render(padToWidth(" No load balancers found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")

		for i := 0; i < 5; i++ {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
	} else {
		lb := m.filtered[m.cursor]

		details := []struct {
			label string
			value string
			style lipgloss.Style
		}{
			{"Name:", lb.Name, NameStyle},
			{"Type:", lb.Type, TypeStyle},
			{"Scheme:", lb.Scheme, MutedStyle},
			{"State:", lb.State, RunningStyle},
			{"DNS:", lb.DNSName, IPStyle},
			{"VPC:", lb.VPCID, IDStyle},
			{"AZs:", strings.Join(lb.AZs, ", "), AZStyle},
		}

		for _, d := range details {
			sb.WriteString(BorderStyle.Render(Vertical))

			labelText := padRight(d.label, lbDetailLabelWidth)
			valueText := d.value

			maxValueWidth := w - 1 - lbDetailLabelWidth
			if runewidth.StringWidth(valueText) > maxValueWidth {
				valueText = runewidth.Truncate(valueText, maxValueWidth, "...")
			}

			plainWidth := 1 + lbDetailLabelWidth + runewidth.StringWidth(valueText)
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

func (m LBModel) renderStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d load balancers", len(m.filtered), len(m.lbs))
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

// SelectLoadBalancer displays an interactive selector for Load Balancers
func SelectLoadBalancer(lbs []pkgtypes.LoadBalancer) (*pkgtypes.LoadBalancer, error) {
	if len(lbs) == 0 {
		return nil, fmt.Errorf("no load balancers available")
	}

	m := NewLBModel(lbs)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(LBModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
