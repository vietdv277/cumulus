package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// VMAction represents the action to take on the selected VM.
type VMAction int

const (
	VMActionConnect VMAction = iota
	VMActionStart
	VMActionStop
)

const (
	vmColWidthID    = 21
	vmColWidthState = 10
	vmColWidthType  = 12
	vmColWidthZone  = 16
	// cursor(3) + ID(21) + sp(2) + State(10) + sp(2) + Type(12) + sp(2) + Zone(16) + sp(2) = 70
	vmFixedWidth = 3 + vmColWidthID + 2 + vmColWidthState + 2 + vmColWidthType + 2 + vmColWidthZone + 2
)

// VMModel is the bubbletea model for interactive VM selection.
type VMModel struct {
	vms          []pkgtypes.VM
	filtered     []pkgtypes.VM
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.VM
	action       VMAction
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
	colWidths    []int // [ID, State, Type, Zone, Name]
}

func newVMModel(vms []pkgtypes.VM) VMModel {
	m := VMModel{
		vms:       vms,
		filtered:  vms,
		cursor:    0,
		offset:    0,
		search:    "",
		termWidth: 80,
	}
	m.calculateVMWidths()
	return m
}

func (m *VMModel) calculateVMWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}

	nameWidth := m.contentWidth - vmFixedWidth
	if nameWidth < 10 {
		nameWidth = 10
	}
	m.colWidths = []int{vmColWidthID, vmColWidthState, vmColWidthType, vmColWidthZone, nameWidth}
}

// Init implements tea.Model.
func (m VMModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model.
func (m VMModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.calculateVMWidths()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			m.cancelled = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				selected := m.filtered[m.cursor]
				m.selected = &selected
				m.action = VMActionConnect
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

		case tea.KeyCtrlS:
			if len(m.filtered) > 0 {
				selected := m.filtered[m.cursor]
				m.selected = &selected
				m.action = VMActionStart
				m.quitting = true
				return m, tea.Quit
			}

		case tea.KeyCtrlX:
			if len(m.filtered) > 0 {
				selected := m.filtered[m.cursor]
				m.selected = &selected
				m.action = VMActionStop
				m.quitting = true
				return m, tea.Quit
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterVMs()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterVMs()
		}
	}

	return m, nil
}

func (m *VMModel) filterVMs() {
	if m.search == "" {
		m.filtered = m.vms
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, vm := range m.vms {
			if strings.Contains(strings.ToLower(vm.Name), query) ||
				strings.Contains(strings.ToLower(vm.ID), query) ||
				strings.Contains(strings.ToLower(vm.PrivateIP), query) ||
				strings.Contains(strings.ToLower(vm.Type), query) ||
				strings.Contains(strings.ToLower(vm.Zone), query) {
				m.filtered = append(m.filtered, vm)
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

// View implements tea.Model.
func (m VMModel) View() string {
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

	// VM list
	visibleEnd := m.offset + listHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}
	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderVMRow(i))
	}
	// Fill remaining rows
	for i := visibleEnd; i < m.offset+listHeight; i++ {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(strings.Repeat(" ", w))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	// Empty line before separator
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
	sb.WriteString(m.renderVMDetailsPanel())

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	// Status bar
	sb.WriteString(m.renderVMStatusBar())

	return sb.String()
}

func (m VMModel) renderVMRow(idx int) string {
	var sb strings.Builder
	vm := m.filtered[idx]
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	// Cursor (3 chars)
	if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	// ID
	idText := padRight(vm.ID, m.colWidths[0])
	line.WriteString(IDStyle.Render(idText))
	line.WriteString("  ")
	plainWidth += m.colWidths[0] + 2

	// State
	stateText := formatStatePlain(string(vm.State), m.colWidths[1])
	line.WriteString(formatStateStyled(string(vm.State), stateText))
	line.WriteString("  ")
	plainWidth += m.colWidths[1] + 2

	// Type
	typeText := padRight(vm.Type, m.colWidths[2])
	line.WriteString(TypeStyle.Render(typeText))
	line.WriteString("  ")
	plainWidth += m.colWidths[2] + 2

	// Zone
	zoneText := padRight(vm.Zone, m.colWidths[3])
	line.WriteString(AZStyle.Render(zoneText))
	line.WriteString("  ")
	plainWidth += m.colWidths[3] + 2

	// Name (dynamic width)
	nameText := padRight(vm.Name, m.colWidths[4])
	line.WriteString(NameStyle.Render(nameText))
	plainWidth += m.colWidths[4]

	// Pad remaining space
	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m VMModel) renderVMDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" VM Details", w)))
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
		sb.WriteString(MutedStyle.Render(padToWidth(" No VMs found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
		// 10 empty lines to match the filled panel height (10 detail rows + 1 trailing)
		for i := 0; i < 10; i++ {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	vm := m.filtered[m.cursor]

	// Build state display string with indicator
	stateStr := string(vm.State)
	stateDisplay := "○ " + stateStr
	switch stateStr {
	case "running":
		stateDisplay = "● " + stateStr
	case "pending", "stopping":
		stateDisplay = "◐ " + stateStr
	}

	igLabel := "ASG:"
	if vm.Provider == "gcp" {
		igLabel = "IG:"
	}

	details := []struct {
		label string
		value string
		style lipgloss.Style
	}{
		{"ID:", vm.ID, IDStyle},
		{"Name:", vm.Name, NameStyle},
		{"State:", stateDisplay, getStateStyle(stateStr)},
		{"Type:", vm.Type, TypeStyle},
		{"Zone:", vm.Zone, AZStyle},
		{"Private IP:", vm.PrivateIP, IPStyle},
		{"Public IP:", formatOptional(vm.PublicIP), IPStyle},
		{igLabel, formatOptional(vm.ASG), ASGStyle},
		{"Launched:", vm.LaunchedAt.Format("2006-01-02 15:04:05"), MutedStyle},
		{"Provider:", vm.Provider, vmProviderStyle(vm.Provider)},
	}

	for _, d := range details {
		sb.WriteString(BorderStyle.Render(Vertical))

		labelText := padRight(d.label, detailLabelWidth)
		valueText := d.value
		maxValueWidth := w - 1 - detailLabelWidth
		if runewidth.StringWidth(valueText) > maxValueWidth {
			valueText = runewidth.Truncate(valueText, maxValueWidth, "...")
		}

		plainWidth := 1 + detailLabelWidth + runewidth.StringWidth(valueText)
		line := MutedStyle.Render(" "+labelText) + d.style.Render(valueText)
		if plainWidth < w {
			line += strings.Repeat(" ", w-plainWidth)
		}

		sb.WriteString(line)
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	// Trailing empty line
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m VMModel) renderVMStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2 // include border chars

	countInfo := fmt.Sprintf("  %d/%d VMs", len(m.filtered), len(m.vms))
	hintsPlain := "[Enter:connect] [^S:start] [^X:stop] [Esc:quit]"

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

func vmProviderStyle(p string) lipgloss.Style {
	switch p {
	case "aws":
		return AWSStyle
	case "gcp":
		return GCPStyle
	default:
		return MutedStyle
	}
}

// SelectVM runs the interactive VM selector TUI and returns the selected VM and action.
func SelectVM(vms []pkgtypes.VM) (*pkgtypes.VM, VMAction, error) {
	if len(vms) == 0 {
		return nil, VMActionConnect, fmt.Errorf("no VMs available")
	}

	m := newVMModel(vms)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, VMActionConnect, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(VMModel)
	if result.cancelled {
		return nil, VMActionConnect, fmt.Errorf("selection cancelled")
	}

	return result.selected, result.action, nil
}
