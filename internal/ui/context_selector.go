package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/vietdv277/cumulus/internal/config"
)

// contextItem holds display data for a single context entry.
type contextItem struct {
	name    string
	ctx     *config.Context
	current bool
}

// ContextModel is the bubbletea model for interactive context selection.
type ContextModel struct {
	items        []contextItem
	filtered     []contextItem
	cursor       int
	offset       int
	search       string
	selected     string
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
	colWidths    []int // [Name, Provider, Credential, Region]
}

func newContextModel(items []contextItem) ContextModel {
	m := ContextModel{
		items:     items,
		filtered:  items,
		termWidth: 80,
	}
	m.calculateContextWidths()
	return m
}

func (m *ContextModel) calculateContextWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}

	// Compute minimum widths from actual content
	provW := runewidth.StringWidth("AWS") // minimum = 3
	credW := 10
	regW := 10
	for _, item := range m.items {
		cred := item.ctx.Profile
		if item.ctx.Project != "" {
			cred = item.ctx.Project
		}
		region := item.ctx.Region
		if region == "" {
			region = "-"
		}
		provW = max(provW, runewidth.StringWidth(strings.ToUpper(item.ctx.Provider)))
		credW = max(credW, runewidth.StringWidth(cred))
		regW = max(regW, runewidth.StringWidth(region))
	}

	// cursor+marker(3) + name(dynamic) + sp(2) + prov + sp(2) + cred + sp(2) + reg
	fixedW := 3 + 2 + provW + 2 + credW + 2 + regW
	nameW := m.contentWidth - fixedW
	if nameW < 10 {
		nameW = 10
	}

	m.colWidths = []int{nameW, provW, credW, regW}
}

// Init implements tea.Model.
func (m ContextModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model.
func (m ContextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.calculateContextWidths()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			m.cancelled = true
			return m, tea.Quit

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.selected = m.filtered[m.cursor].name
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
				m.filterContexts()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterContexts()
		}
	}

	return m, nil
}

func (m *ContextModel) filterContexts() {
	if m.search == "" {
		m.filtered = m.items
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, item := range m.items {
			if strings.Contains(strings.ToLower(item.name), query) {
				m.filtered = append(m.filtered, item)
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
func (m ContextModel) View() string {
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

	// Context list
	visibleEnd := m.offset + listHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}
	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderContextRow(i))
	}
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
	sb.WriteString(m.renderContextDetailsPanel())

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	// Status bar
	sb.WriteString(m.renderContextStatusBar())

	return sb.String()
}

func (m ContextModel) renderContextRow(idx int) string {
	item := m.filtered[idx]
	w := m.contentWidth

	var sb strings.Builder
	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	// 3-char prefix: space + cursor(>) + current-marker(*)
	cursor := " "
	if idx == m.cursor {
		cursor = ">"
	}
	marker := " "
	if item.current {
		marker = "*"
	}
	line.WriteString(" " + cursor + marker)
	plainWidth += 3

	// Name
	nameText := padRight(item.name, m.colWidths[0])
	if item.current {
		line.WriteString(RunningStyle.Render(nameText))
	} else {
		line.WriteString(NameStyle.Render(nameText))
	}
	line.WriteString("  ")
	plainWidth += m.colWidths[0] + 2

	// Provider
	provPlain := strings.ToUpper(item.ctx.Provider)
	provText := padRight(provPlain, m.colWidths[1])
	line.WriteString(vmProviderStyle(item.ctx.Provider).Render(provText))
	line.WriteString("  ")
	plainWidth += m.colWidths[1] + 2

	// Credential (profile or project)
	cred := item.ctx.Profile
	if item.ctx.Project != "" {
		cred = item.ctx.Project
	}
	credText := padRight(cred, m.colWidths[2])
	line.WriteString(MutedStyle.Render(credText))
	line.WriteString("  ")
	plainWidth += m.colWidths[2] + 2

	// Region
	region := item.ctx.Region
	if region == "" {
		region = "-"
	}
	regText := padRight(region, m.colWidths[3])
	line.WriteString(AZStyle.Render(regText))
	plainWidth += m.colWidths[3]

	// Pad remaining space
	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m ContextModel) renderContextDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	// Header
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" Context Details", w)))
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
		sb.WriteString(MutedStyle.Render(padToWidth(" No contexts found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
		// 4 empty lines to match filled panel height (4 detail rows + 1 trailing)
		for range 4 {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	item := m.filtered[m.cursor]

	cred := item.ctx.Profile
	credLabel := "Profile:"
	if item.ctx.Project != "" {
		cred = item.ctx.Project
		credLabel = "Project:"
	}
	region := item.ctx.Region
	if region == "" {
		region = "-"
	}

	details := []struct {
		label string
		value string
		style lipgloss.Style
	}{
		{"Context:", item.name, NameStyle},
		{"Provider:", strings.ToUpper(item.ctx.Provider), vmProviderStyle(item.ctx.Provider)},
		{credLabel, cred, MutedStyle},
		{"Region:", region, AZStyle},
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

func (m ContextModel) renderContextStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d contexts", len(m.filtered), len(m.items))
	hintsPlain := "[Enter:select] [Esc:quit]"

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

// SelectContext runs the interactive context selector TUI and returns the selected context name.
// The current context is pre-highlighted in the list.
func SelectContext(contexts map[string]*config.Context, current string) (string, error) {
	if len(contexts) == 0 {
		return "", fmt.Errorf("no contexts available")
	}

	names := make([]string, 0, len(contexts))
	for name := range contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]contextItem, len(names))
	for i, name := range names {
		items[i] = contextItem{
			name:    name,
			ctx:     contexts[name],
			current: name == current,
		}
	}

	m := newContextModel(items)

	// Pre-position cursor on the current context
	for i, item := range items {
		if item.current {
			m.cursor = i
			break
		}
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(ContextModel)
	if result.cancelled {
		return "", fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
