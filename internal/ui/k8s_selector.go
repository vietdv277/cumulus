package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// K8sModel is the bubbletea model for interactive cluster selection.
type K8sModel struct {
	clusters     []pkgtypes.K8sCluster
	filtered     []pkgtypes.K8sCluster
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.K8sCluster
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
	colWidths    []int // [Name, Version, Status, Region, Provider]
}

func newK8sModel(clusters []pkgtypes.K8sCluster) K8sModel {
	m := K8sModel{
		clusters:  clusters,
		filtered:  clusters,
		termWidth: 80,
	}
	m.calculateK8sWidths()
	return m
}

func (m *K8sModel) calculateK8sWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}

	versionW := 8
	statusW := 10
	regionW := 10
	provW := 3
	for _, c := range m.clusters {
		versionW = max(versionW, runewidth.StringWidth(c.Version))
		statusW = max(statusW, runewidth.StringWidth(c.Status))
		regionW = max(regionW, runewidth.StringWidth(c.Region))
		provW = max(provW, runewidth.StringWidth(strings.ToUpper(c.Provider)))
	}

	// cursor(3) + name + sp(2) + version + sp(2) + status + sp(2) + region + sp(2) + prov
	fixedW := 3 + 2 + versionW + 2 + statusW + 2 + regionW + 2 + provW
	nameW := m.contentWidth - fixedW
	if nameW < 10 {
		nameW = 10
	}

	m.colWidths = []int{nameW, versionW, statusW, regionW, provW}
}

// Init implements tea.Model.
func (m K8sModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model.
func (m K8sModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.calculateK8sWidths()
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
				m.filterK8s()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterK8s()
		}
	}

	return m, nil
}

func (m *K8sModel) filterK8s() {
	if m.search == "" {
		m.filtered = m.clusters
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, c := range m.clusters {
			if strings.Contains(strings.ToLower(c.Name), query) ||
				strings.Contains(strings.ToLower(c.Region), query) ||
				strings.Contains(strings.ToLower(c.Version), query) {
				m.filtered = append(m.filtered, c)
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
func (m K8sModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(TopLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(TopRight))
	sb.WriteString("\n")

	searchLine := " > " + m.search
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(NameStyle.Render(padToWidth(searchLine, w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	visibleEnd := m.offset + listHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}
	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderK8sRow(i))
	}
	for i := visibleEnd; i < m.offset+listHeight; i++ {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(strings.Repeat(" ", w))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(strings.Repeat(" ", w))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	sb.WriteString(BorderStyle.Render(LeftT))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(RightT))
	sb.WriteString("\n")

	sb.WriteString(m.renderK8sDetailsPanel())

	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	sb.WriteString(m.renderK8sStatusBar())

	return sb.String()
}

func (m K8sModel) renderK8sRow(idx int) string {
	c := m.filtered[idx]
	w := m.contentWidth

	var sb strings.Builder
	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	nameText := padRight(c.Name, m.colWidths[0])
	line.WriteString(NameStyle.Render(nameText))
	line.WriteString("  ")
	plainWidth += m.colWidths[0] + 2

	versionText := padRight(c.Version, m.colWidths[1])
	line.WriteString(MutedStyle.Render(versionText))
	line.WriteString("  ")
	plainWidth += m.colWidths[1] + 2

	statusText := padRight(c.Status, m.colWidths[2])
	line.WriteString(clusterStatusStyle(c.Status).Render(statusText))
	line.WriteString("  ")
	plainWidth += m.colWidths[2] + 2

	regionText := padRight(c.Region, m.colWidths[3])
	line.WriteString(AZStyle.Render(regionText))
	line.WriteString("  ")
	plainWidth += m.colWidths[3] + 2

	provText := padRight(strings.ToUpper(c.Provider), m.colWidths[4])
	line.WriteString(vmProviderStyle(c.Provider).Render(provText))
	plainWidth += m.colWidths[4]

	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m K8sModel) renderK8sDetailsPanel() string {
	var sb strings.Builder
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" Cluster Details", w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	sb.WriteString(BorderStyle.Render(Vertical))
	underline := " " + strings.Repeat("─", 20)
	sb.WriteString(MutedStyle.Render(padToWidth(underline, w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	const detailRows = 6

	if len(m.filtered) == 0 {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(MutedStyle.Render(padToWidth(" No clusters found", w)))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
		for range detailRows {
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString(strings.Repeat(" ", w))
			sb.WriteString(BorderStyle.Render(Vertical))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	c := m.filtered[m.cursor]

	details := []struct {
		label string
		value string
		style lipgloss.Style
	}{
		{"Name:", c.Name, NameStyle},
		{"Version:", c.Version, MutedStyle},
		{"Status:", c.Status, clusterStatusStyle(c.Status)},
		{"Region:", c.Region, AZStyle},
		{"Endpoint:", formatOptional(c.Endpoint), IPStyle},
		{"Provider:", strings.ToUpper(c.Provider), vmProviderStyle(c.Provider)},
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

	return sb.String()
}

func (m K8sModel) renderK8sStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d clusters", len(m.filtered), len(m.clusters))
	hintsPlain := "[Enter:use] [Esc:quit]"

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

func clusterStatusStyle(status string) lipgloss.Style {
	switch strings.ToUpper(status) {
	case "ACTIVE", "RUNNING":
		return RunningStyle
	case "CREATING", "UPDATING", "PROVISIONING", "RECONCILING":
		return PendingStyle
	default:
		return StoppedStyle
	}
}

// SelectK8sCluster runs the interactive cluster selector TUI and returns the
// cluster the user picked. Selection triggers a kubeconfig update in the caller.
func SelectK8sCluster(clusters []pkgtypes.K8sCluster) (*pkgtypes.K8sCluster, error) {
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no clusters available")
	}

	m := newK8sModel(clusters)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(K8sModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}
