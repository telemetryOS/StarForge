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

// Styles shared with the command/status output.
var (
	tuiBg     = lipgloss.Color("#0f1419")
	tuiBgDark = lipgloss.Color("#0b0f12")

	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6dde3"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff6b6b"))
	progressStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ccfd8"))
	successStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91"))
	warningStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d8b45f"))
	headingStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d6dde3")).Background(tuiBg)
	subheadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8fbfd0")).Background(tuiBg)
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790")).Background(tuiBg)
	valueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6dde3")).Background(tuiBg)
	rowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6dde3")).Background(tuiBg)
	rowSelected   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d6dde3")).Background(lipgloss.Color("#162027"))
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#26323b")).Background(tuiBg).Padding(0, 1)
	panelFocus    = panelStyle.BorderForeground(lipgloss.Color("#7ccf91"))
	badgeStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0b0f12")).Background(lipgloss.Color("#9ccfd8")).Padding(0, 1)
	dangerBadge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0b0f12")).Background(lipgloss.Color("#d8b45f")).Padding(0, 1)

	// TUI chrome
	tuiTitleStar  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ccf91")).Background(tuiBgDark)
	tuiTitleForge = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ccfd8")).Background(tuiBgDark)
	tuiTitleCmd   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790")).Background(tuiBgDark).PaddingLeft(1)
	tuiTitlePad   = lipgloss.NewStyle().Background(tuiBgDark)
	tuiFooterDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8790")).Background(tuiBgDark)
	tuiFooterKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ccfd8")).Background(tuiBgDark)
	tuiChromeBg   = lipgloss.NewStyle().Background(tuiBg)
	tuiContentPad = lipgloss.NewStyle().Padding(1, 2).Background(tuiBg)
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

	contentHeight := max(m.height-4, 1) // title + footer + vertical padding
	styled := tuiContentPad.
		Width(max(m.width-4, 1)).
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
	return m.screen("Preparing Installer", m.planColumn(), panelStyle.Width(m.mainWidth()).Render(strings.Join([]string{
		subheadStyle.Render("Reading install media"),
		"",
		normalStyle.Render("Connecting to the local install service."),
		dimStyle.Render("Payloads and eligible disks will appear here once discovery completes."),
	}, "\n")))
}

func (m model) viewPayloadSelect() string {
	rows := []string{subheadStyle.Render("Choose the system image")}
	for i, p := range m.payloads {
		line := fmt.Sprintf("%-24s %10s  %2d partitions", p.Name, diskutil.FormatSize(payloadSize(p)), len(p.Partitions))
		if i == m.payloadCursor {
			rows = append(rows, rowSelected.Width(m.mainInnerWidth()).Render("  "+line))
		} else {
			rows = append(rows, rowStyle.Width(m.mainInnerWidth()).Render("  "+line))
		}
		if p.Description != "" {
			rows = append(rows, dimStyle.Render("    "+truncate(p.Description, m.mainInnerWidth()-4)))
		}
	}
	return m.screen("Select Image", m.planColumn(), panelStyle.Width(m.mainWidth()).Render(strings.Join(rows, "\n")))
}

func (m model) viewDiskSelect() string {
	if m.payloadCursor >= len(m.payloads) {
		return ""
	}

	rows := []string{
		subheadStyle.Render("Choose target disk"),
		dimStyle.Render("The installer USB and mounted system disks are hidden."),
		"",
	}
	if len(m.disks) == 0 {
		rows = append(rows,
			warningStyle.Render("No available disks found."),
			dimStyle.Render("Check that the target drive is attached and not mounted."),
		)
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

			line := fmt.Sprintf("%s/dev/%-8s  %-28s  %10s  %s",
				cursor,
				d.Name,
				truncate(dmodel, 28),
				diskutil.FormatSize(d.Size),
				transportLabel(d.Transport))
			if i == m.diskCursor {
				rows = append(rows, rowSelected.Width(m.mainInnerWidth()).Render(line))
			} else {
				rows = append(rows, style.Render(line))
			}
		}
	}

	return m.screen("Select Target", m.planColumn(), panelStyle.Width(m.mainWidth()).Render(strings.Join(rows, "\n")))
}

func (m model) viewConfirm() string {
	if m.payloadCursor >= len(m.payloads) || m.diskCursor >= len(m.disks) {
		return ""
	}
	payload := m.payloads[m.payloadCursor]
	disk := m.disks[m.diskCursor]

	body := panelFocus.Width(m.mainWidth()).Render(strings.Join([]string{
		dangerBadge.Render("READY TO WRITE"),
		"",
		metaLine("Image", payload.Name),
		metaLine("Image size", diskutil.FormatSize(payloadSize(payload))),
		metaLine("Target", fmt.Sprintf("/dev/%s", disk.Name)),
		metaLine("Disk model", emptyDefault(disk.Model, "Unknown")),
		metaLine("Disk size", diskutil.FormatSize(disk.Size)),
		"",
		warningStyle.Render("The selected disk will be repartitioned and overwritten."),
		dimStyle.Render("Press y to start. Press n or esc to go back."),
	}, "\n"))
	return m.screen("Confirm Install", m.planColumn(), body)
}

func (m model) viewProgress() string {
	barWidth := max(min(m.mainInnerWidth()-8, 68), 20)
	body := panelStyle.Width(m.mainWidth()).Render(strings.Join([]string{
		subheadStyle.Render("Writing image"),
		m.installSummary(),
		"",
		progressBar(m.progress, barWidth) + fmt.Sprintf(" %3.0f%%", m.progress*100),
		"",
		m.phaseList(),
		"",
		subheadStyle.Render("Installer log"),
		m.logTail(m.height - 22),
	}, "\n"))
	return m.screen("Installing", m.planColumn(), body)
}

func (m model) viewComplete() string {
	body := panelFocus.Width(m.mainWidth()).Render(strings.Join([]string{
		successStyle.Render("Installation complete"),
		m.installSummary(),
		"",
		normalStyle.Render("The system image was written and configured."),
		dimStyle.Render("Press r to reboot into the installed OS, or q to stay here."),
		"",
		subheadStyle.Render("Final log lines"),
		m.logTail(m.height - 20),
	}, "\n"))
	return m.screen("Ready to Reboot", m.planColumn(), body)
}

func (m model) viewError() string {
	lines := []string{errorStyle.Render("Installation cannot continue.")}
	if m.loadErr != nil {
		lines = append(lines, "", normalStyle.Render(fmt.Sprintf("%v", m.loadErr)))
	}
	if m.installErr != "" && (m.loadErr == nil || m.installErr != m.loadErr.Error()) {
		lines = append(lines, errorStyle.Render(fmt.Sprintf("Detail: %s", m.installErr)))
	}
	if len(m.logLines) > 0 {
		lines = append(lines, "", subheadStyle.Render("Installer log"), m.logTail(m.height-18))
	}

	return m.screen("Install Error", m.planColumn(), panelStyle.Width(m.mainWidth()).Render(strings.Join(lines, "\n")))
}

func (m model) screen(title, left, main string) string {
	header := headingStyle.Render(title)
	rule := dimStyle.Render(strings.Repeat("─", max(min(m.width-8, 96), 12)))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, tuiChromeBg.Render("  "), main)
	if m.width < 80 {
		body = left + "\n\n" + main
	}
	return strings.Join([]string{header, rule, "", body}, "\n")
}

func (m model) planColumn() string {
	width := m.planWidth()
	lines := []string{
		subheadStyle.Render("Install Plan"),
		"",
		planRow("1", "Image", m.payloadName(), m.phase == phasePayloadSelect),
		planRow("2", "Target", m.diskName(), m.phase == phaseDiskSelect),
		planRow("3", "Confirm", confirmState(m.phase), m.phase == phaseConfirm),
		planRow("4", "Write", statusText(m.status), m.phase == phaseProgress),
	}
	return panelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func planRow(num, label, value string, active bool) string {
	marker := " " + num + " "
	if active {
		marker = badgeStyle.Render(num)
	}
	if value == "" {
		value = "pending"
	}
	return fmt.Sprintf("%s %s\n   %s", marker, labelStyle.Render(label), dimStyle.Render(value))
}

func confirmState(p phase) string {
	switch p {
	case phaseConfirm:
		return "review selection"
	case phaseProgress, phaseComplete:
		return "accepted"
	default:
		return "pending"
	}
}

func (m model) installSummary() string {
	return metaLine("Image", m.payloadName()) + "\n" +
		metaLine("Target", m.diskName()) + "\n" +
		metaLine("Phase", statusText(m.status))
}

func (m model) phaseList() string {
	phases := []string{"partitioning", "copying", "configuring", "complete"}
	current := statusText(m.status)
	var rows []string
	for _, p := range phases {
		prefix := "  "
		style := dimStyle
		if p == current {
			prefix = "> "
			style = progressStyle
		} else if phaseDone(p, current) {
			prefix = "✓ "
			style = successStyle
		}
		rows = append(rows, style.Render(prefix+p))
	}
	return strings.Join(rows, "\n")
}

func phaseDone(phaseName, current string) bool {
	order := map[string]int{
		"partitioning": 1,
		"copying":      2,
		"configuring":  3,
		"complete":     4,
	}
	return order[phaseName] < order[current]
}

func (m model) logTail(maxLines int) string {
	if maxLines < 4 {
		maxLines = 4
	}
	if len(m.logLines) == 0 {
		return dimStyle.Render("Waiting for installer output...")
	}
	start := 0
	if len(m.logLines) > maxLines {
		start = len(m.logLines) - maxLines
	}
	var rows []string
	for _, line := range m.logLines[start:] {
		rows = append(rows, dimStyle.Render(truncate(line, max(m.mainInnerWidth()-2, 8))))
	}
	return strings.Join(rows, "\n")
}

func (m model) payloadName() string {
	if m.payloadCursor < len(m.payloads) {
		return m.payloads[m.payloadCursor].Name
	}
	return ""
}

func (m model) diskName() string {
	if m.diskCursor < len(m.disks) {
		return "/dev/" + m.disks[m.diskCursor].Name
	}
	return ""
}

func (m model) planWidth() int {
	if m.width < 80 {
		return max(m.width-8, 24)
	}
	return 28
}

func (m model) mainWidth() int {
	if m.width < 80 {
		return max(m.width-8, 24)
	}
	return max(m.width-m.planWidth()-12, 32)
}

func (m model) mainInnerWidth() int {
	return max(m.mainWidth()-4, 16)
}

func payloadSize(p installer.PayloadManifest) uint64 {
	var total uint64
	for _, part := range p.Partitions {
		total += part.Size
	}
	return total
}

func transportLabel(transport string) string {
	if transport == "" {
		return "disk"
	}
	return transport
}

func emptyDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func metaLine(label, value string) string {
	return fmt.Sprintf("%s %s",
		labelStyle.Width(10).Render(label),
		valueStyle.Render(value))
}

func statusText(status string) string {
	if status == "" {
		return "starting"
	}
	return status
}

func progressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	return labelStyle.Render("[") +
		progressStyle.Render(strings.Repeat("█", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled)) +
		labelStyle.Render("]")
}

func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:0]
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
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
