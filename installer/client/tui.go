package client

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
)

// Styles — ember palette.
var (
	tuiBg     = lipgloss.Color("#1c1408")
	tuiBgDark = lipgloss.Color("#120d08")

	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a028"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0d0a0"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a5538"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e84848"))
	progressStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e8b830"))
	successStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8a028"))
	warningStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e89030"))

	// TUI chrome
	tuiTitleStar  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e8b830")).Background(tuiBgDark)
	tuiTitleForge = lipgloss.NewStyle().Foreground(lipgloss.Color("#d48820")).Background(tuiBgDark)
	tuiTitleCmd   = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050")).Background(tuiBgDark).PaddingLeft(1)
	tuiTitlePad   = lipgloss.NewStyle().Background(tuiBgDark)
	tuiFooterDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050")).Background(tuiBgDark)
	tuiFooterKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e89030")).Background(tuiBgDark)
	tuiChromeBg   = lipgloss.NewStyle().Background(tuiBg)
	tuiContentPad = lipgloss.NewStyle().PaddingLeft(2).Background(tuiBg)
)

// phase tracks the current TUI screen.
type phase int

const (
	phaseLoading phase = iota
	phasePayloadSelect
	phaseDiskSelect
	phaseConfirm
	phaseProgress
	phaseComplete
	phaseError
)

// model is the bubbletea model.
type model struct {
	client     *Client
	phase      phase
	unattended bool

	// Loading
	loadErr error

	// Payload selection
	payloads      []installer.PayloadManifest
	payloadCursor int

	// Disk selection
	disks      []diskutil.Disk
	diskCursor int

	// Confirmation
	confirmed bool

	// Progress
	installID  string
	status     string
	progress   float64
	logLines   []string
	logOffset  int
	installErr string

	// Window size
	width  int
	height int
}

// Messages
type loadedMsg struct {
	payloads []installer.PayloadManifest
	disks    []diskutil.Disk
	err      error
}

type installStartedMsg struct {
	id  string
	err error
}

type installUpdateMsg struct {
	status   string
	progress float64
	lines    []string
	offset   int
	err      string
}

type tickMsg struct{}

// RunTUI starts the interactive installer TUI.
func RunTUI(serverURL string, unattended bool) error {
	c := NewClient(serverURL)
	m := model{
		client:     c,
		phase:      phaseLoading,
		unattended: unattended,
	}
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return loadData(m.client)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.phase != phaseProgress {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case loadedMsg:
		if msg.err != nil {
			m.phase = phaseError
			m.loadErr = msg.err
			return m, nil
		}
		m.payloads = msg.payloads
		m.disks = msg.disks
		if len(m.payloads) == 0 {
			m.phase = phaseError
			m.loadErr = fmt.Errorf("no payloads found on the server")
			return m, nil
		}
		if m.unattended {
			if len(m.disks) == 0 {
				m.phase = phaseError
				m.loadErr = fmt.Errorf("unattended: no available disks found")
				return m, nil
			}
			m.payloadCursor = 0
			m.diskCursor = 0
			m.confirmed = true
			return m, startInstall(m.client, m.payloads[0].Name, m.disks[0].Name)
		}
		if len(m.payloads) == 1 {
			// Skip payload selection if only one
			m.payloadCursor = 0
			m.phase = phaseDiskSelect
		} else {
			m.phase = phasePayloadSelect
		}
		return m, nil

	case installStartedMsg:
		if msg.err != nil {
			m.phase = phaseError
			m.loadErr = msg.err
			return m, nil
		}
		m.installID = msg.id
		m.phase = phaseProgress
		return m, pollInstall(m.client, m.installID, m.logOffset)

	case installUpdateMsg:
		m.status = msg.status
		m.progress = msg.progress
		if len(msg.lines) > 0 {
			m.logLines = append(m.logLines, msg.lines...)
			m.logOffset = msg.offset
		}
		if msg.err != "" {
			m.installErr = msg.err
		}

		if msg.status == "complete" {
			m.phase = phaseComplete
			if m.unattended {
				if err := m.client.Reboot(); err != nil {
					m.loadErr = fmt.Errorf("reboot failed: %w", err)
					m.phase = phaseError
				}
				return m, tea.Quit
			}
			return m, nil
		}
		if msg.status == "failed" {
			m.phase = phaseError
			m.loadErr = fmt.Errorf("%s", msg.err)
			return m, nil
		}
		return m, pollInstall(m.client, m.installID, m.logOffset)

	case tickMsg:
		return m, pollInstall(m.client, m.installID, m.logOffset)
	}

	// Phase-specific key handling
	switch m.phase {
	case phasePayloadSelect:
		return m.updatePayloadSelect(msg)
	case phaseDiskSelect:
		return m.updateDiskSelect(msg)
	case phaseConfirm:
		return m.updateConfirm(msg)
	case phaseComplete:
		return m.updateComplete(msg)
	}

	return m, nil
}

func (m model) updatePayloadSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.payloadCursor > 0 {
				m.payloadCursor--
			}
		case "down", "j":
			if m.payloadCursor < len(m.payloads)-1 {
				m.payloadCursor++
			}
		case "enter":
			m.phase = phaseDiskSelect
		}
	}
	return m, nil
}

func (m model) updateDiskSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.diskCursor > 0 {
				m.diskCursor--
			}
		case "down", "j":
			if m.diskCursor < len(m.disks)-1 {
				m.diskCursor++
			}
		case "enter":
			if len(m.disks) > 0 {
				m.phase = phaseConfirm
			}
		case "backspace", "esc":
			if len(m.payloads) > 1 {
				m.phase = phasePayloadSelect
			}
		}
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y":
			if m.payloadCursor >= len(m.payloads) || m.diskCursor >= len(m.disks) {
				return m, nil
			}
			m.confirmed = true
			payload := m.payloads[m.payloadCursor].Name
			disk := m.disks[m.diskCursor].Name
			return m, startInstall(m.client, payload, disk)
		case "n", "N", "backspace", "esc":
			m.phase = phaseDiskSelect
		}
	}
	return m, nil
}

func (m model) updateComplete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "r", "R":
			if err := m.client.Reboot(); err != nil {
				m.loadErr = fmt.Errorf("reboot failed: %w", err)
				m.phase = phaseError
			}
			return m, tea.Quit
		case "q", "Q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true

	if m.width == 0 {
		v.Content = "\n  Initializing..."
		return v
	}

	var b strings.Builder

	// Title bar
	b.WriteString(tuiTitleBar("install", m.width))
	b.WriteByte('\n')

	// Content area
	var content, footer string
	switch m.phase {
	case phaseLoading:
		content = m.viewLoading()
		footer = m.footerHelp("q quit")
	case phasePayloadSelect:
		content = m.viewPayloadSelect()
		footer = m.footerHelp("↑/↓ select  enter confirm  q quit")
	case phaseDiskSelect:
		content = m.viewDiskSelect()
		footer = m.footerHelp("↑/↓ select  enter confirm  esc back  q quit")
	case phaseConfirm:
		content = m.viewConfirm()
		footer = m.footerHelp("y confirm  n cancel  q quit")
	case phaseProgress:
		content = m.viewProgress()
		footer = m.footerProgress()
	case phaseComplete:
		content = m.viewComplete()
		footer = m.footerHelp("r reboot  q quit")
	case phaseError:
		content = m.viewError()
		footer = m.footerHelp("q quit")
	}

	contentHeight := max(m.height-2, 1) // title + footer
	styled := tuiContentPad.
		Width(m.width).
		Height(contentHeight).
		Render(content)
	b.WriteString(styled)
	b.WriteByte('\n')

	// Footer bar
	b.WriteString(footer)

	v.Content = tuiFillScreen(b.String(), m.width, m.height)
	return v
}

func (m model) viewLoading() string {
	return "\n Loading..."
}

func (m model) viewPayloadSelect() string {
	var b strings.Builder
	b.WriteString("\nSelect a payload to install:\n\n")

	for i, p := range m.payloads {
		cursor := "  "
		style := normalStyle
		if i == m.payloadCursor {
			cursor = "> "
			style = selectedStyle
		}

		var totalSize uint64
		for _, part := range p.Partitions {
			totalSize += part.Size
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n",
			cursor,
			style.Render(p.Name),
			dimStyle.Render(fmt.Sprintf("(%s)", diskutil.FormatSize(totalSize)))))
		if p.Description != "" {
			b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(p.Description)))
		}
	}

	return b.String()
}

func (m model) viewDiskSelect() string {
	var b strings.Builder
	if m.payloadCursor >= len(m.payloads) {
		return ""
	}
	b.WriteString(fmt.Sprintf("\nPayload: %s\n\n", selectedStyle.Render(m.payloads[m.payloadCursor].Name)))
	b.WriteString("Select a target disk:\n\n")

	if len(m.disks) == 0 {
		b.WriteString(warningStyle.Render("No available disks found."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("All disks are either mounted or the installer USB."))
		b.WriteString("\n")
	} else {
		for i, d := range m.disks {
			cursor := "  "
			style := normalStyle
			if i == m.diskCursor {
				cursor = "> "
				style = selectedStyle
			}

			dmodel := d.Model
			if dmodel == "" {
				dmodel = "Unknown"
			}

			b.WriteString(fmt.Sprintf("%s%s  %s  %s  %s\n",
				cursor,
				style.Render(fmt.Sprintf("/dev/%-8s", d.Name)),
				dimStyle.Render(fmt.Sprintf("%-20s", dmodel)),
				dimStyle.Render(diskutil.FormatSize(d.Size)),
				dimStyle.Render(d.Transport)))
		}
	}

	return b.String()
}

func (m model) viewConfirm() string {
	if m.payloadCursor >= len(m.payloads) || m.diskCursor >= len(m.disks) {
		return ""
	}
	payload := m.payloads[m.payloadCursor]
	disk := m.disks[m.diskCursor]

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(warningStyle.Render("WARNING: All data on the target disk will be destroyed!"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Payload: %s\n", selectedStyle.Render(payload.Name)))
	b.WriteString(fmt.Sprintf("Disk:    %s (%s, %s)\n",
		selectedStyle.Render("/dev/"+disk.Name), disk.Model, diskutil.FormatSize(disk.Size)))
	b.WriteString("\n")
	b.WriteString("Proceed with installation? ")
	b.WriteString(selectedStyle.Render("[y/N]"))
	return b.String()
}

func (m model) viewProgress() string {
	var b strings.Builder

	// Progress bar
	barWidth := 40
	if m.width > 60 {
		barWidth = m.width - 24
	}
	filled := int(m.progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	b.WriteString(fmt.Sprintf("\n[%s] %.0f%%\n", progressStyle.Render(bar), m.progress*100))
	b.WriteString(fmt.Sprintf("Status: %s\n\n", normalStyle.Render(m.status)))

	// Log lines (show last N that fit)
	maxLines := m.height - 8
	if maxLines < 5 {
		maxLines = 5
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}
	for _, line := range m.logLines[start:] {
		b.WriteString(dimStyle.Render(line))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m model) viewComplete() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(successStyle.Render("Installation complete!"))
	b.WriteString("\n\n")

	// Show log
	maxLines := m.height - 8
	if maxLines < 5 {
		maxLines = 5
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}
	for _, line := range m.logLines[start:] {
		b.WriteString(dimStyle.Render(line))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m model) viewError() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(errorStyle.Render("Error: "))
	if m.loadErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("%v", m.loadErr)))
	}
	b.WriteString("\n")
	if m.installErr != "" && (m.loadErr == nil || m.installErr != m.loadErr.Error()) {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Detail: %s", m.installErr)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(m.logLines) > 0 {
		b.WriteString(dimStyle.Render("Log:"))
		b.WriteString("\n")
		maxLines := m.height - 10
		if maxLines < 5 {
			maxLines = 5
		}
		start := 0
		if len(m.logLines) > maxLines {
			start = len(m.logLines) - maxLines
		}
		for _, line := range m.logLines[start:] {
			b.WriteString(dimStyle.Render(line))
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// TUI chrome helpers — matching build TUI appearance.

func tuiTitleBar(cmd string, width int) string {
	title := tuiTitleStar.Render("STAR") +
		tuiTitleForge.Render("FORGE") +
		tuiTitleCmd.Render(cmd)
	titleW := lipgloss.Width(title)
	if titleW < width {
		title += tuiTitlePad.Render(strings.Repeat(" ", width-titleW))
	}
	return title
}

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

func tuiFooterBar(left, right string, width int) string {
	fSp := lipgloss.NewStyle().Background(tuiBgDark)
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := max(width-leftW-rightW, 0)
	return left + fSp.Render(strings.Repeat(" ", gap)) + right
}

func (m model) footerHelp(help string) string {
	return tuiFooterBar(tuiFooterDim.Render(" "+help), "", m.width)
}

func (m model) footerProgress() string {
	left := tuiFooterDim.Render(" installing...")
	right := tuiFooterKey.Render(fmt.Sprintf("%3.f%%", m.progress*100)) +
		tuiFooterDim.Render(" ")
	return tuiFooterBar(left, right, m.width)
}

// Commands

func loadData(c *Client) tea.Cmd {
	return func() tea.Msg {
		payloads, err := c.ListPayloads()
		if err != nil {
			return loadedMsg{err: fmt.Errorf("connecting to server: %w", err)}
		}

		disks, err := c.ListDisks()
		if err != nil {
			return loadedMsg{err: fmt.Errorf("listing disks: %w", err)}
		}

		return loadedMsg{payloads: payloads, disks: disks}
	}
}

func startInstall(c *Client, payload, disk string) tea.Cmd {
	return func() tea.Msg {
		inst, err := c.StartInstallation(payload, disk)
		if err != nil {
			return installStartedMsg{err: err}
		}
		return installStartedMsg{id: inst.ID}
	}
}

func pollInstall(c *Client, id string, offset int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)

		inst, err := c.GetInstallation(id)
		if err != nil {
			return installUpdateMsg{status: "failed", err: err.Error()}
		}

		lines, newOffset, _ := c.GetLog(id, offset)

		return installUpdateMsg{
			status:   inst.Status,
			progress: inst.Progress,
			lines:    lines,
			offset:   newOffset,
			err:      inst.Error,
		}
	}
}
