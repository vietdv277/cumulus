package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

const (
	profileListHeight = 10
)

// ProfileModel represents the bubbletea model for profile selection
type ProfileModel struct {
	profiles     []pkgtypes.AWSProfile
	filtered     []pkgtypes.AWSProfile
	cursor       int
	offset       int
	search       string
	selected     *pkgtypes.AWSProfile
	quitting     bool
	cancelled    bool
	termWidth    int
	contentWidth int
	activeProfile string
}

// NewProfileModel creates a new profile selector model
func NewProfileModel(profiles []pkgtypes.AWSProfile, activeProfile string) ProfileModel {
	m := ProfileModel{
		profiles:      profiles,
		filtered:      profiles,
		cursor:        0,
		offset:        0,
		search:        "",
		termWidth:     80,
		activeProfile: activeProfile,
	}
	m.calculateWidths()
	return m
}

func (m *ProfileModel) calculateWidths() {
	m.contentWidth = m.termWidth - 2
	if m.contentWidth < minWidth {
		m.contentWidth = minWidth
	}
	if m.contentWidth > maxWidth {
		m.contentWidth = maxWidth
	}
}

// Init implements tea.Model
func (m ProfileModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update implements tea.Model
func (m ProfileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				if m.cursor >= m.offset+profileListHeight {
					m.offset = m.cursor - profileListHeight + 1
				}
			}

		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.filterProfiles()
			}

		case tea.KeyRunes:
			m.search += string(msg.Runes)
			m.filterProfiles()
		}
	}

	return m, nil
}

func (m *ProfileModel) filterProfiles() {
	if m.search == "" {
		m.filtered = m.profiles
	} else {
		query := strings.ToLower(m.search)
		m.filtered = nil
		for _, p := range m.profiles {
			if strings.Contains(strings.ToLower(p.Name), query) ||
				strings.Contains(strings.ToLower(p.Region), query) {
				m.filtered = append(m.filtered, p)
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
func (m ProfileModel) View() string {
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

	// Title
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString(HeaderStyle.Render(padToWidth(" Select AWS Profile", w)))
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	// Separator
	sb.WriteString(BorderStyle.Render(LeftT))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(RightT))
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

	// Profile list
	visibleEnd := m.offset + profileListHeight
	if visibleEnd > len(m.filtered) {
		visibleEnd = len(m.filtered)
	}

	for i := m.offset; i < visibleEnd; i++ {
		sb.WriteString(m.renderProfileRow(i))
	}

	// Fill remaining lines
	for i := len(m.filtered); i < m.offset+profileListHeight; i++ {
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString(strings.Repeat(" ", w))
		sb.WriteString(BorderStyle.Render(Vertical))
		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BorderStyle.Render(BottomLeft))
	sb.WriteString(BorderStyle.Render(strings.Repeat(Horizontal, w)))
	sb.WriteString(BorderStyle.Render(BottomRight))
	sb.WriteString("\n")

	// Status bar
	sb.WriteString(m.renderStatusBar())

	return sb.String()
}

func (m ProfileModel) renderProfileRow(idx int) string {
	var sb strings.Builder
	profile := m.filtered[idx]
	w := m.contentWidth

	sb.WriteString(BorderStyle.Render(Vertical))

	var line strings.Builder
	plainWidth := 0

	// Active indicator
	if profile.Name == m.activeProfile {
		line.WriteString(" ● ")
	} else if idx == m.cursor {
		line.WriteString(" > ")
	} else {
		line.WriteString("   ")
	}
	plainWidth += 3

	// Name
	nameWidth := 30
	nameText := padRight(profile.Name, nameWidth)
	if profile.Name == m.activeProfile {
		line.WriteString(RunningStyle.Render(nameText))
	} else {
		line.WriteString(NameStyle.Render(nameText))
	}
	line.WriteString("  ")
	plainWidth += nameWidth + 2

	// Region
	regionWidth := 20
	regionText := profile.Region
	if regionText == "" {
		regionText = "-"
	}
	regionText = padRight(regionText, regionWidth)
	line.WriteString(MutedStyle.Render(regionText))
	plainWidth += regionWidth

	// Pad to fill
	if plainWidth < w {
		line.WriteString(strings.Repeat(" ", w-plainWidth))
	}

	sb.WriteString(line.String())
	sb.WriteString(BorderStyle.Render(Vertical))
	sb.WriteString("\n")

	return sb.String()
}

func (m ProfileModel) renderStatusBar() string {
	var sb strings.Builder
	w := m.contentWidth + 2

	countInfo := fmt.Sprintf("  %d/%d profiles", len(m.filtered), len(m.profiles))
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

// SelectProfile displays an interactive selector for AWS profiles
func SelectProfile(profiles []pkgtypes.AWSProfile, activeProfile string) (*pkgtypes.AWSProfile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles available")
	}

	m := NewProfileModel(profiles, activeProfile)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("error running selector: %w", err)
	}

	result := finalModel.(ProfileModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}

	return result.selected, nil
}

// PrintProfileTable prints profiles in a styled table
func PrintProfileTable(profiles []pkgtypes.AWSProfile, activeProfile string) {
	headers := []string{"", "Name", "Region", "Source"}

	// Calculate name column width
	nameWidth := len(headers[1])
	for _, p := range profiles {
		if len(p.Name) > nameWidth {
			nameWidth = len(p.Name)
		}
	}

	colWidths := []int{3, nameWidth, 20, 12}

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
	for _, profile := range profiles {
		sb.WriteString(BorderStyle.Render(Vertical))

		// Active indicator
		activeCell := "   "
		if profile.Name == activeProfile {
			activeCell = " ● "
		}
		sb.WriteString(RunningStyle.Render(padRight(activeCell, colWidths[0]+2)))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Name
		cell := " " + padRight(profile.Name, colWidths[1]) + " "
		if profile.Name == activeProfile {
			sb.WriteString(RunningStyle.Render(cell))
		} else {
			sb.WriteString(NameStyle.Render(cell))
		}
		sb.WriteString(BorderStyle.Render(Vertical))

		// Region
		region := profile.Region
		if region == "" {
			region = "-"
		}
		cell = " " + padRight(region, colWidths[2]) + " "
		sb.WriteString(MutedStyle.Render(cell))
		sb.WriteString(BorderStyle.Render(Vertical))

		// Source
		cell = " " + padRight(profile.Source, colWidths[3]) + " "
		sb.WriteString(MutedStyle.Render(cell))
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
	fmt.Printf("  %d profiles\n", len(profiles))
}
