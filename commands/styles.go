package commands

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Shared lipgloss styles for command output and TUI chrome.
var (
	cmdHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8fbfd0"))
	cmdSuccess = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91"))
	cmdDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))

	// Status command
	statusBuilt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91"))
	statusNotBuilt = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))

	// Inspect command
	inspectHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8fbfd0"))
	inspectDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))
	inspectOverridden = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("#7d8790"))
	inspectActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7ccf91"))

	// Inspect TUI sidebar
	sidebarSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91"))
	sidebarNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6dde3"))
	sidebarEmpty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))
	sidebarCount    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))
	sidebarBorder   = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("#2a3138"))

	// Inspect TUI search
	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ccfd8"))
	searchMatchStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d8b45f"))
)

// TUI chrome.
var (
	tuiBg     = lipgloss.Color("#0f1419")
	tuiBgDark = lipgloss.Color("#0b0f12")

	tuiTitleStar = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7ccf91")).
			Background(tuiBgDark)

	tuiTitleForge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ccfd8")).
			Background(tuiBgDark)

	tuiTitleCmd = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7d8790")).
			Background(tuiBgDark).
			PaddingLeft(1)

	tuiTitlePad = lipgloss.NewStyle().Background(tuiBgDark)

	tuiFooterDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7d8790")).
			Background(tuiBgDark)

	tuiFooterAccent = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ccfd8")).
			Background(tuiBgDark)

	tuiChromeBg = lipgloss.NewStyle().Background(tuiBg)
)

// tuiFillScreen pads every line to width and fills remaining height with bg.
func tuiFillScreen(content string, width, height int) string {
	fill := tuiChromeBg.Render
	lines := strings.Split(content, "\n")

	var out strings.Builder
	for i, line := range lines {
		visible := lipgloss.Width(line)
		if visible < width {
			line += fill(strings.Repeat(" ", width-visible))
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}

	blankLine := fill(strings.Repeat(" ", width))
	for i := len(lines); i < height; i++ {
		out.WriteByte('\n')
		out.WriteString(blankLine)
	}

	return out.String()
}

// tuiTitleBar renders a full-width title bar: "STARFORGE <cmd> <detail>".
func tuiTitleBar(cmd, detail string, width int) string {
	title := tuiTitleStar.Render("STAR") +
		tuiTitleForge.Render("FORGE") +
		tuiTitleCmd.Render(cmd+" "+detail)
	titleW := lipgloss.Width(title)
	if titleW < width {
		title += tuiTitlePad.Render(strings.Repeat(" ", width-titleW))
	}
	return title
}

// tuiFooterBar renders a full-width footer bar with left help and optional right content.
func tuiFooterBar(left, right string, width int) string {
	fSp := lipgloss.NewStyle().Background(tuiBgDark)
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := max(width-leftW-rightW, 0)
	return left + fSp.Render(strings.Repeat(" ", gap)) + right
}
