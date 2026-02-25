package client

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
)

// Styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	progressStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	successStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	warningStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
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
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return loadData(m.client)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
				m.client.Reboot()
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
	case tea.KeyMsg:
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
	case tea.KeyMsg:
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
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
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
	case tea.KeyMsg:
		switch msg.String() {
		case "r", "R":
			m.client.Reboot()
			return m, tea.Quit
		case "q", "Q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.phase {
	case phaseLoading:
		return m.viewLoading()
	case phasePayloadSelect:
		return m.viewPayloadSelect()
	case phaseDiskSelect:
		return m.viewDiskSelect()
	case phaseConfirm:
		return m.viewConfirm()
	case phaseProgress:
		return m.viewProgress()
	case phaseComplete:
		return m.viewComplete()
	case phaseError:
		return m.viewError()
	}
	return ""
}

func (m model) viewLoading() string {
	return "\n  Loading...\n"
}

func (m model) viewPayloadSelect() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")
	b.WriteString("  Select a payload to install:\n\n")

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

		b.WriteString(fmt.Sprintf("  %s%s %s\n",
			cursor,
			style.Render(p.Name),
			dimStyle.Render(fmt.Sprintf("(%s)", diskutil.FormatSize(totalSize)))))
		if p.Description != "" {
			b.WriteString(fmt.Sprintf("      %s\n", dimStyle.Render(p.Description)))
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Use arrow keys to select, Enter to confirm, q to quit"))
	b.WriteString("\n")
	return b.String()
}

func (m model) viewDiskSelect() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Payload: %s\n\n", selectedStyle.Render(m.payloads[m.payloadCursor].Name)))
	b.WriteString("  Select a target disk:\n\n")

	if len(m.disks) == 0 {
		b.WriteString(warningStyle.Render("  No available disks found."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  All disks are either mounted or the installer USB."))
		b.WriteString("\n")
	} else {
		for i, d := range m.disks {
			cursor := "  "
			style := normalStyle
			if i == m.diskCursor {
				cursor = "> "
				style = selectedStyle
			}

			model := d.Model
			if model == "" {
				model = "Unknown"
			}

			b.WriteString(fmt.Sprintf("  %s%s  %s  %s  %s\n",
				cursor,
				style.Render(fmt.Sprintf("/dev/%-8s", d.Name)),
				dimStyle.Render(fmt.Sprintf("%-20s", model)),
				dimStyle.Render(diskutil.FormatSize(d.Size)),
				dimStyle.Render(d.Transport)))
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Use arrow keys to select, Enter to confirm, Esc to go back"))
	b.WriteString("\n")
	return b.String()
}

func (m model) viewConfirm() string {
	payload := m.payloads[m.payloadCursor]
	disk := m.disks[m.diskCursor]

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")
	b.WriteString(warningStyle.Render("  WARNING: All data on the target disk will be destroyed!"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Payload: %s\n", selectedStyle.Render(payload.Name)))
	b.WriteString(fmt.Sprintf("  Disk:    %s (%s, %s)\n",
		selectedStyle.Render("/dev/"+disk.Name), disk.Model, diskutil.FormatSize(disk.Size)))
	b.WriteString("\n")
	b.WriteString("  Proceed with installation? ")
	b.WriteString(selectedStyle.Render("[y/N]"))
	b.WriteString("\n")
	return b.String()
}

func (m model) viewProgress() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")

	// Progress bar
	barWidth := 40
	if m.width > 60 {
		barWidth = m.width - 20
	}
	filled := int(m.progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	b.WriteString(fmt.Sprintf("  [%s] %.0f%%\n", progressStyle.Render(bar), m.progress*100))
	b.WriteString(fmt.Sprintf("  Status: %s\n\n", m.status))

	// Log lines (show last N that fit)
	maxLines := m.height - 10
	if maxLines < 5 {
		maxLines = 5
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}
	for _, line := range m.logLines[start:] {
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(line)))
	}

	return b.String()
}

func (m model) viewComplete() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")
	b.WriteString(successStyle.Render("  Installation complete!"))
	b.WriteString("\n\n")

	// Show log
	maxLines := m.height - 10
	if maxLines < 5 {
		maxLines = 5
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}
	for _, line := range m.logLines[start:] {
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(line)))
	}

	b.WriteString("\n")
	b.WriteString("  Press ")
	b.WriteString(selectedStyle.Render("r"))
	b.WriteString(" to reboot, ")
	b.WriteString(selectedStyle.Render("q"))
	b.WriteString(" to quit\n")
	return b.String()
}

func (m model) viewError() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  StarForge Installer"))
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render("  Error: "))
	if m.loadErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("%v", m.loadErr)))
	}
	b.WriteString("\n")
	if m.installErr != "" && (m.loadErr == nil || m.installErr != m.loadErr.Error()) {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Detail: %s", m.installErr)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(m.logLines) > 0 {
		b.WriteString(dimStyle.Render("  Log:"))
		b.WriteString("\n")
		maxLines := m.height - 12
		if maxLines < 5 {
			maxLines = 5
		}
		start := 0
		if len(m.logLines) > maxLines {
			start = len(m.logLines) - maxLines
		}
		for _, line := range m.logLines[start:] {
			b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(line)))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  Press q to quit"))
	b.WriteString("\n")
	return b.String()
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
