package ui

import "github.com/charmbracelet/lipgloss"

// Styles holds all lipgloss renderers used by the table.
type Styles struct {
	// Usage-level cell styles (applied to CPU% and MEM% values)
	OK   lipgloss.Style
	Warn lipgloss.Style
	Crit lipgloss.Style

	// Row-level styles
	Header    lipgloss.Style
	PodName   lipgloss.Style
	Container lipgloss.Style
	PlainCell   lipgloss.Style
	Divider     lipgloss.Style

	// Status line (namespace header + refresh timestamp)
	StatusLine  lipgloss.Style // bold label text
	StatusValue lipgloss.Style // non-bold value text
	Dim         lipgloss.Style // dimmed/faint text
}

// DefaultStyles returns the standard color theme.
// When noColor is true all styles are left unstyled so output is plain text.
func DefaultStyles(noColor bool) Styles {
	if noColor {
		return Styles{
			OK: lipgloss.NewStyle(), Warn: lipgloss.NewStyle(), Crit: lipgloss.NewStyle(),
			Header: lipgloss.NewStyle(), PodName: lipgloss.NewStyle(),
			Container: lipgloss.NewStyle(), PlainCell: lipgloss.NewStyle(),
			Divider: lipgloss.NewStyle(),
			StatusLine: lipgloss.NewStyle(), StatusValue: lipgloss.NewStyle(),
			Dim:        lipgloss.NewStyle(),
		}
	}

	return Styles{
		OK:         lipgloss.NewStyle().Foreground(lipgloss.Color("2")),           // green
		Warn:       lipgloss.NewStyle().Foreground(lipgloss.Color("3")),           // yellow
		Crit:       lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true), // bold red

		Header:  lipgloss.NewStyle().Bold(true),
		PodName: lipgloss.NewStyle().Bold(true),
		Container:  lipgloss.NewStyle(),
		PlainCell:  lipgloss.NewStyle(),
		Divider:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")), // dark gray

		StatusLine:  lipgloss.NewStyle().Bold(true),
		StatusValue: lipgloss.NewStyle(),
		Dim:        lipgloss.NewStyle().Faint(true),
	}
}
