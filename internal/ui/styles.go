package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Box drawing characters
const (
	TopLeft     = "╭"
	TopRight    = "╮"
	BottomLeft  = "╰"
	BottomRight = "╯"
	Horizontal  = "─"
	Vertical    = "│"
	LeftT       = "├"
	RightT      = "┤"
	TopT        = "┬"
	BottomT     = "┴"
	Cross       = "┼"
)

// Color palette
const (
	ColorBorder  = "240"
	ColorHeader  = "252"
	ColorID      = "214"
	ColorName    = "81"
	ColorIP      = "252"
	ColorType    = "252"
	ColorAZ      = "252"
	ColorASG     = "245"
	ColorRunning = "82"
	ColorStopped = "245"
	ColorPending = "214"
	ColorMuted   = "240"
	ColorHint    = "245"
)

// Shared styles
var (
	BorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBorder))
	HeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorHeader))
	IDStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorID))
	NameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorName))
	IPStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorIP))
	TypeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorType))
	AZStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAZ))
	ASGStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorASG))
	RunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRunning))
	StoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorStopped))
	PendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorPending))
	MutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))
	HintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorHint))
)

// padRight pads a string to the specified display width using runewidth
func padRight(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
