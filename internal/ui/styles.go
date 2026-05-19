package ui

import "github.com/charmbracelet/lipgloss"

// Styles holds all lipgloss renderers used by the table.
type Styles struct {
	// Usage-level cell styles (applied to CPU% and MEM% values)
	OK   lipgloss.Style
	Warn lipgloss.Style
	Crit lipgloss.Style

	// Row-level styles
	Header      lipgloss.Style
	PodName     lipgloss.Style
	PodRestart  lipgloss.Style // pod name with non-zero restarts
	Container   lipgloss.Style
	Sidecar     lipgloss.Style // dimmed for istio-proxy / envoy / linkerd-proxy
	PlainCell   lipgloss.Style

	// Status line (namespace header + refresh timestamp)
	StatusLine lipgloss.Style
}

// DefaultStyles returns the standard color theme.
// When noColor is true all styles are left unstyled so output is plain text.
func DefaultStyles(noColor bool) Styles {
	if noColor {
		return Styles{
			OK: lipgloss.NewStyle(), Warn: lipgloss.NewStyle(), Crit: lipgloss.NewStyle(),
			Header: lipgloss.NewStyle(), PodName: lipgloss.NewStyle(), PodRestart: lipgloss.NewStyle(),
			Container: lipgloss.NewStyle(), Sidecar: lipgloss.NewStyle(), PlainCell: lipgloss.NewStyle(),
			StatusLine: lipgloss.NewStyle(),
		}
	}

	return Styles{
		OK:         lipgloss.NewStyle().Foreground(lipgloss.Color("2")),           // green
		Warn:       lipgloss.NewStyle().Foreground(lipgloss.Color("3")),           // yellow
		Crit:       lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true), // bold red

		Header:     lipgloss.NewStyle().Bold(true).Underline(true),
		PodName:    lipgloss.NewStyle().Bold(true),
		PodRestart: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // bold yellow
		Container:  lipgloss.NewStyle(),
		Sidecar:    lipgloss.NewStyle().Faint(true),
		PlainCell:  lipgloss.NewStyle(),

		StatusLine: lipgloss.NewStyle().Bold(true),
	}
}
