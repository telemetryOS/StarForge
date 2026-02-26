package commands

import "charm.land/lipgloss/v2"

// Shared lipgloss styles for command output — ember palette.
var (
	cmdHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d07030"))
	cmdSuccess = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a028"))
	cmdDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))

	// Status command
	statusBuilt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a028"))
	statusNotBuilt = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))

	// Inspect command
	inspectHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d07030"))
	inspectDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))
	inspectOverridden = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("#b08050"))
	inspectActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("#c8a028"))

	// Inspect TUI sidebar
	sidebarSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a028"))
	sidebarNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0d0a0"))
	sidebarEmpty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))
	sidebarCount    = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))
	sidebarBorder   = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("#3d2a18"))

	// Inspect TUI search
	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d48820"))
	searchMatchStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e8b830"))
)
