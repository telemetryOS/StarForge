package engine

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

// Color palette — warm ember tones: yellow ↔ red ↔ black.
var (
	colorText   = lipgloss.Color("#f0d0a0") // warm peach-cream
	colorDim    = lipgloss.Color("#7a5538") // muted orange
	colorSubtle = lipgloss.Color("#3d2a18") // dark orange-brown

	colorAccent    = lipgloss.Color("#e8b830") // golden amber (primary accent)
	colorEmphasis  = lipgloss.Color("#d48820") // warm orange (emphasis)
	colorHeading   = lipgloss.Color("#d07030") // burnt orange (headers)

	colorSuccess   = lipgloss.Color("#c8a028") // warm gold (success)
	colorError     = lipgloss.Color("#e84848") // bright red (errors)
	colorHighlight = lipgloss.Color("#e89030") // amber (accents)

	colorMuted     = lipgloss.Color("#c89868") // warm orange-tan (step actions)

	colorBg     = lipgloss.Color("#1c1408") // warm orange-black
	colorBgDark = lipgloss.Color("#120d08") // deep orange-black
)

// Styles used by the output system and engine code.
var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorHeading)
	phaseStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorEmphasis)
	accentStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	stepStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(colorSuccess)
	cachedStyle  = lipgloss.NewStyle().Foreground(colorSuccess)
	dimStyle     = lipgloss.NewStyle().Foreground(colorDim)
	failStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorError)

	// Brighter variants for post-TUI summary (renders on user's terminal bg).
	summaryDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#b08050"))
)

// TUI-specific styles.
var (
	titleBarStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			Background(colorBgDark).
			Padding(0, 1)

	titleStarStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Background(colorBgDark)

	titleForgeStyle = lipgloss.NewStyle().
			Foreground(colorEmphasis).
			Background(colorBgDark)

	titleCmdStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Background(colorBgDark).
			Padding(0, 1)

	viewportBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSubtle).
				BorderBackground(colorBg)

	footerBarStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorBgDark)

	footerDimStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Background(colorBgDark)

	footerAccentStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSuccess).
				Background(colorBgDark)

	footerScrollStyle = lipgloss.NewStyle().
				Foreground(colorHighlight).
				Background(colorBgDark)

	stageRunStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Background(colorBg)
	treeStyle     = lipgloss.NewStyle().Foreground(colorSubtle).Background(colorBg)

	// Chrome helpers — bg-aware variants for TUI chrome area only.
	chromeBg       = lipgloss.NewStyle().Background(colorBg)
	chromeText     = lipgloss.NewStyle().Foreground(colorText).Background(colorBg)
	chromeDim      = dimStyle.Background(colorBg)
	chromeSuccess  = successStyle.Background(colorBg)
	chromeError    = failStyle.Background(colorBg)
	chromeAccent   = accentStyle.Background(colorBg)
	chromeEmphasis = phaseStyle.Background(colorBg)
	chromeCached   = cachedStyle.Background(colorBg)
)

// Output manages all build output to terminal and log file.
type Output struct {
	program     *tea.Program
	log         *os.File
	interactive bool
	done        bool // true after bubbletea program exits
	mu          sync.Mutex
}

// Package-level instance, set by InitOutput().
var out *Output

// InitOutput creates the global output instance. buildDir is used for the
// log file path; pass "" for no log file. commandName and targetName are
// displayed in the TUI header. If an Output already exists (e.g. nested
// builds for installer payloads), returns the existing instance.
func InitOutput(buildDir, commandName, targetName string) (*Output, error) {
	if out != nil {
		return out, nil
	}

	o := &Output{
		interactive: isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
	}

	// Open build log
	if buildDir != "" {
		logPath := fmt.Sprintf("%s/build.log", buildDir)
		f, err := os.Create(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: build log disabled: %v\n", err)
		} else {
			fmt.Fprintf(f, "Build started: %s\n\n", time.Now().Format(time.RFC3339))
			o.log = f
		}
	}

	if o.interactive {
		s := spinner.New()
		s.Spinner = spinner.MiniDot
		s.Style = lipgloss.NewStyle().Foreground(colorAccent).Background(colorBg)

		vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
		vp.MouseWheelEnabled = true
		vp.SoftWrap = true

		model := buildModel{
			commandName: commandName,
			targetName:  targetName,
			spinner:     s,
			activePhase: -1,
			activeLayer: -1,
			viewport:    vp,
			logLines:    make([]string, 0, 256),
			autoScroll:  true,
			totalStart:  time.Now(),
		}
		for i, name := range PhaseNames {
			model.phases[i] = phaseInfo{name: phaseShortName(name)}
		}
		o.program = tea.NewProgram(model,
			tea.WithOutput(os.Stderr),
		)
	}

	out = o
	return o, nil
}

// Run executes fn within the bubbletea lifecycle. In interactive mode,
// bubbletea runs in the foreground while fn runs in a goroutine. In
// non-interactive mode, fn is called directly.
func (o *Output) Run(fn func() error) error {
	if o == nil || !o.interactive || o.program == nil {
		return fn()
	}

	var fnErr error
	go func() {
		fnErr = fn()
		o.program.Send(buildDoneMsg{})
	}()

	finalModel, err := o.program.Run()

	o.mu.Lock()
	o.done = true
	o.program = nil
	o.mu.Unlock()

	if err != nil {
		return err
	}

	// Print summary to stderr after alt screen exits
	if m, ok := finalModel.(buildModel); ok {
		m.printSummary(fnErr)
	}

	return fnErr
}

// Close writes the log footer and resets the package-level output.
func (o *Output) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.log != nil {
		fmt.Fprintf(o.log, "\nBuild ended: %s\n", time.Now().Format(time.RFC3339))
		o.log.Close()
		o.log = nil
	}
	if out == o {
		out = nil
	}
}

// --- Output methods ---

// Header prints a bold header line.
func (o *Output) Header(text string) {
	styled := headerStyle.Render(text)
	o.println(styled, text)
}

// Phase prints a phase name and updates the dashboard.
func (o *Output) Phase(name string) {
	styled := "  " + phaseStyle.Render(name)
	o.println(styled, "  "+name)
	if idx := phaseIndex(name); idx >= 0 {
		o.sendMsg(startPhaseMsg{index: idx})
	}
}

// PhaseCached prints a cached phase and updates the dashboard.
func (o *Output) PhaseCached(name string) {
	styled := fmt.Sprintf("  %s %s", phaseStyle.Render(name), cachedStyle.Render("cached"))
	o.println(styled, fmt.Sprintf("  %s cached", name))
	if idx := phaseIndex(name); idx >= 0 {
		o.sendMsg(phaseCachedMsg{index: idx})
	}
}

// PhaseComplete marks a phase as completed with its elapsed time.
func (o *Output) PhaseComplete(name string, elapsed time.Duration) {
	if idx := phaseIndex(name); idx >= 0 {
		o.sendMsg(endPhaseMsg{index: idx, elapsed: elapsed})
	}
}

// PhaseFailed marks a phase as failed in the dashboard.
func (o *Output) PhaseFailed(name string) {
	if idx := phaseIndex(name); idx >= 0 {
		o.sendMsg(phaseFailedMsg{index: idx})
	}
}

// StartStage marks a build stage as running.
func (o *Output) StartStage(s Stage) {
	o.sendMsg(startStageMsg{stage: s})
}

// EndStage marks a build stage as complete.
func (o *Output) EndStage(s Stage, elapsed time.Duration) {
	o.sendMsg(endStageMsg{stage: s, elapsed: elapsed})
}

// CollectLayer marks a layer as being processed during Collect.
func (o *Output) CollectLayer(index int, name string) {
	o.sendMsg(collectLayerMsg{index: index, name: name})
}

// CollectLayerDone marks a layer as complete.
func (o *Output) CollectLayerDone(index int, elapsed time.Duration) {
	o.sendMsg(collectLayerDoneMsg{index: index, elapsed: elapsed})
}

// Info prints an indented info line.
func (o *Output) Info(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	o.println("    "+text, "    "+text)
}

// SubInfo prints a doubly-indented dim info line.
func (o *Output) SubInfo(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	styled := "      " + dimStyle.Render(text)
	o.println(styled, "      "+text)
}

// Blank prints an empty line.
func (o *Output) Blank() {
	o.println("", "")
}

// Success prints a green bold success line.
func (o *Output) Success(text string) {
	styled := successStyle.Render(text)
	o.println(styled, text)
}

// Warning prints a warning line.
func (o *Output) Warning(text string) {
	line := "  warning: " + text
	o.println(line, line)
}

// Styled prints an already-styled line (for callers that build their own styling).
func (o *Output) Styled(styled, plain string) {
	o.println(styled, plain)
}

// RunWithSpinner runs fn with an animated spinner (interactive) or
// plain "label..." → "✓ label [time]" output (non-interactive).
func (o *Output) RunWithSpinner(label string, fn func() error) error {
	if o == nil {
		return fn()
	}
	start := time.Now()

	if o.isActive() {
		o.program.Send(startSpinnerMsg{label: label})
		err := fn()
		elapsed := time.Since(start)
		o.program.Send(stopSpinnerMsg{})
		if err != nil {
			o.println(
				fmt.Sprintf("    ✗ %s  %s", label, dimStyle.Render(formatDuration(elapsed))),
				fmt.Sprintf("    ✗ %s [%s]", label, formatDuration(elapsed)),
			)
			return err
		}
		o.println(
			fmt.Sprintf("    ✓ %s  %s", label, dimStyle.Render(formatDuration(elapsed))),
			fmt.Sprintf("    ✓ %s [%s]", label, formatDuration(elapsed)),
		)
		return nil
	}

	// Non-interactive fallback
	fmt.Printf("    %s... ", label)
	o.logLine(fmt.Sprintf("    %s...", label))
	err := fn()
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("✗ [%s]\n", formatDuration(elapsed))
		o.logLine(fmt.Sprintf("    ✗ %s [%s]", label, formatDuration(elapsed)))
		return err
	}
	fmt.Printf("✓ [%s]\n", formatDuration(elapsed))
	o.logLine(fmt.Sprintf("    ✓ %s [%s]", label, formatDuration(elapsed)))
	return nil
}

// ProcessWriter returns a writer for subprocess output that should be visible
// (pacstrap, scripts). Each complete line is printed via the output system.
func (o *Output) ProcessWriter() io.Writer {
	if o == nil {
		return io.Discard
	}
	return &lineWriter{output: o, prefix: "      "}
}

// LogWriter returns a writer for subprocess output that should be suppressed
// from terminal output. Writes to log file only.
func (o *Output) LogWriter() io.Writer {
	if o == nil {
		return io.Discard
	}
	return &logOnlyWriter{output: o}
}

// --- Exported helpers for use by commands package ---

// OutputSuccess prints a success message through the global output. Safe
// to call when out is nil (falls back to fmt.Println).
func OutputSuccess(text string) {
	if out != nil {
		out.Blank()
		out.Success(text)
	} else {
		fmt.Println()
		fmt.Println(text)
	}
}

// OutputInfo prints an info message through the global output.
func OutputInfo(format string, args ...any) {
	if out != nil {
		out.Info(format, args...)
	} else {
		fmt.Printf("    "+format+"\n", args...)
	}
}

// --- Internal helpers ---

// maxLogLines is the maximum number of log lines kept in the viewport buffer.
const maxLogLines = 1000

// isActive returns true if the TUI program is running and accepting messages.
func (o *Output) isActive() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.interactive && !o.done && o.program != nil
}

// sendMsg sends a message to the bubbletea program if interactive and running.
func (o *Output) sendMsg(msg tea.Msg) {
	if o.isActive() {
		o.program.Send(msg)
	}
}

// phaseIndex returns the index (0-8) for a PhaseNames entry, or -1 if not found.
func phaseIndex(name string) int {
	for i, pn := range PhaseNames {
		if pn == name {
			return i
		}
	}
	return -1
}

// phaseShortName strips the "N-" prefix from a phase name for display.
func phaseShortName(name string) string {
	if len(name) > 2 && name[1] == '-' {
		return name[2:]
	}
	return name
}

// println sends a line to the terminal and log file.
func (o *Output) println(styled, plain string) {
	if o == nil {
		return
	}
	if o.isActive() {
		o.program.Send(logLineMsg{line: styled})
	} else {
		fmt.Println(styled)
	}
	o.logLine(plain)
}

// logLine writes a plain text line to the log file.
func (o *Output) logLine(text string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.log != nil {
		fmt.Fprintln(o.log, text)
	}
}

// --- Stage types ---

// Stage represents a high-level build stage.
type Stage int

const (
	StageCollect Stage = iota
	StageBuild
	StagePackage
	StageWrite
	StageAssemble
)

var stageNames = [5]string{"Collect", "Build", "Package", "Write", "Assemble"}

type stageInfo struct {
	name    string
	status  phaseStatus
	start   time.Time
	elapsed time.Duration
}

// --- Bubbletea model ---

type phaseStatus int

const (
	phasePending phaseStatus = iota
	phaseRunning
	phaseCached
	phaseComplete
	phaseFailed
)

type phaseInfo struct {
	name    string
	status  phaseStatus
	elapsed time.Duration
	start   time.Time
}

type startSpinnerMsg struct{ label string }
type stopSpinnerMsg struct{}
type buildDoneMsg struct{}
type startPhaseMsg struct {
	index int
}
type endPhaseMsg struct {
	index   int
	elapsed time.Duration
}
type phaseCachedMsg struct{ index int }
type phaseFailedMsg struct{ index int }

type startStageMsg struct{ stage Stage }
type endStageMsg struct {
	stage   Stage
	elapsed time.Duration
}
type logLineMsg struct{ line string }

// collectLayerMsg tracks a layer being processed during Collect.
type collectLayerMsg struct {
	index int
	name  string
}
type collectLayerDoneMsg struct {
	index   int
	elapsed time.Duration
}

// tickMsg fires periodically to update elapsed time displays.
type tickMsg time.Time

// layerInfo tracks a layer during the Collect stage.
type layerInfo struct {
	name    string
	status  phaseStatus
	start   time.Time
	elapsed time.Duration
}

type buildModel struct {
	commandName string
	targetName  string

	stages     [5]stageInfo
	stageCount int // how many stages have been started (only show these)

	spinner     spinner.Model
	phases      [9]phaseInfo
	activePhase int // -1 when none running

	// Collect stage layer tracking
	layers      []layerInfo
	activeLayer int // -1 when none running

	// Inner spinner (current operation within a phase)
	label  string
	start  time.Time
	active bool

	viewport   viewport.Model
	logLines   []string
	autoScroll bool

	totalStart time.Time
	quitting   bool
	width      int
	height     int
	ready      bool
}

func (m buildModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tickCmd())
}

// tickCmd returns a command that sends a tickMsg every 100ms.
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// chromeHeight returns the number of lines used by non-viewport chrome.
func (m buildModel) chromeHeight() int {
	h := 1 // title bar
	h += m.stageCount
	// Layer tree under Collect
	if len(m.layers) > 0 {
		h += len(m.layers)
	}
	// Phase grid under Build
	if m.stages[StageBuild].status != phasePending {
		h += 5
	}
	h += 2 // viewport border top+bottom
	h += 1 // footer bar
	return h
}

// recalcLayout updates the viewport dimensions based on terminal size and chrome.
func (m *buildModel) recalcLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	vpHeight := max(m.height-m.chromeHeight(), 1)
	vpWidth := max(m.width-2, 1) // account for left+right border
	m.viewport.SetWidth(vpWidth)
	m.viewport.SetHeight(vpHeight)
	m.setViewportContent()
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

// setViewportContent rebuilds the viewport content from logLines.
// Pads with empty lines to fill the viewport height so the gradient
// StyleLineFunc covers the entire visible area (viewport adds fill
// lines after StyleLineFunc, so we must pre-pad).
func (m *buildModel) setViewportContent() {
	content := strings.Join(m.logLines, "\n")
	vpH := m.viewport.Height()
	if vpH > 0 && len(m.logLines) < vpH {
		content += strings.Repeat("\n", vpH-len(m.logLines))
	}
	m.viewport.SetContent(content)
}

func (m buildModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startStageMsg:
		idx := int(msg.stage)
		m.stages[idx] = stageInfo{
			name:   stageNames[idx],
			status: phaseRunning,
			start:  time.Now(),
		}
		if idx+1 > m.stageCount {
			m.stageCount = idx + 1
		}
		m.recalcLayout()
		return m, nil

	case endStageMsg:
		idx := int(msg.stage)
		m.stages[idx].status = phaseComplete
		m.stages[idx].elapsed = msg.elapsed
		// Clear layers when Collect completes (collapse tree)
		if msg.stage == StageCollect {
			m.layers = nil
			m.activeLayer = -1
			m.recalcLayout()
		}
		return m, nil

	case collectLayerMsg:
		// Grow or reuse the layer slice
		for len(m.layers) <= msg.index {
			m.layers = append(m.layers, layerInfo{})
		}
		m.layers[msg.index] = layerInfo{
			name:   msg.name,
			status: phaseRunning,
			start:  time.Now(),
		}
		m.activeLayer = msg.index
		m.recalcLayout()
		return m, nil

	case collectLayerDoneMsg:
		if msg.index < len(m.layers) {
			m.layers[msg.index].status = phaseComplete
			m.layers[msg.index].elapsed = msg.elapsed
			if m.activeLayer == msg.index {
				m.activeLayer = -1
			}
		}
		return m, nil

	case logLineMsg:
		m.logLines = append(m.logLines, msg.line)
		if len(m.logLines) > maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
		}
		m.setViewportContent()
		if m.autoScroll {
			m.viewport.GotoBottom()
		}
		return m, nil

	case startPhaseMsg:
		m.phases[msg.index].status = phaseRunning
		m.phases[msg.index].start = time.Now()
		m.activePhase = msg.index
		m.recalcLayout()
		return m, nil

	case endPhaseMsg:
		m.phases[msg.index].status = phaseComplete
		m.phases[msg.index].elapsed = msg.elapsed
		if m.activePhase == msg.index {
			m.activePhase = -1
		}
		return m, nil

	case phaseCachedMsg:
		m.phases[msg.index].status = phaseCached
		return m, nil

	case phaseFailedMsg:
		m.phases[msg.index].status = phaseFailed
		if m.activePhase == msg.index {
			m.activePhase = -1
		}
		return m, nil

	case startSpinnerMsg:
		m.label = msg.label
		m.start = time.Now()
		m.active = true
		return m, m.spinner.Tick

	case stopSpinnerMsg:
		m.active = false
		m.label = ""
		return m, nil

	case buildDoneMsg:
		m.quitting = true
		return m, tea.Quit

	case tickMsg:
		return m, tickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.recalcLayout()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "pgup":
			m.autoScroll = false
			m.viewport.HalfPageUp()
			return m, nil
		case "pgdown":
			m.viewport.HalfPageDown()
			m.autoScroll = m.viewport.AtBottom()
			return m, nil
		case "home":
			m.autoScroll = false
			m.viewport.GotoTop()
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			m.autoScroll = true
			return m, nil
		}

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		m.autoScroll = m.viewport.AtBottom()
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m buildModel) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	if m.quitting || !m.ready {
		return v
	}

	var b strings.Builder

	// ── Title bar ──────────────────────────────────────
	titleContent := titleStarStyle.Render("STAR") +
		titleForgeStyle.Render("FORGE") +
		titleCmdStyle.Render(m.commandName+" "+m.targetName)
	// Pad to full width
	titleVisible := lipgloss.Width(titleContent)
	if titleVisible < m.width {
		titleContent += titleBarStyle.Render(strings.Repeat(" ", m.width-titleVisible))
	}
	b.WriteString(titleContent)
	b.WriteByte('\n')

	// ── Stages with nested sub-items ─────────────────
	for i := 0; i < m.stageCount; i++ {
		b.WriteString(m.renderStageCell(i))
		b.WriteByte('\n')

		// Layer tree nested under Collect
		if Stage(i) == StageCollect && len(m.layers) > 0 {
			for j, layer := range m.layers {
				connector := "├"
				if j == len(m.layers)-1 {
					connector = "└"
				}

				var icon, name, status string
				switch layer.status {
				case phaseRunning:
					icon = m.spinner.View()
					name = chromeAccent.Render(layer.name)
					status = chromeDim.Render(formatDuration(time.Since(layer.start)))
				case phaseComplete:
					icon = chromeSuccess.Render("✓")
					name = chromeText.Render(layer.name)
					status = chromeDim.Render(formatDuration(layer.elapsed))
				default:
					icon = chromeDim.Render("○")
					name = chromeDim.Render(layer.name)
				}

				cell := icon + chromeBg.Render(" ") + name
				if status != "" {
					cell += chromeBg.Render("  ") + status
				}
				b.WriteString(treeStyle.Render("   "+connector+" "))
				b.WriteString(cell)
				b.WriteByte('\n')
			}
		}

		// Phase grid nested under Build
		if Stage(i) == StageBuild && m.stages[StageBuild].status != phasePending {
			const cellWidth = 26
			for row := range 5 {
				left := m.renderPhaseCell(row, cellWidth)
				right := ""
				ri := row + 5
				if ri < 9 {
					right = m.renderPhaseCell(ri, cellWidth)
				}
				// Tree connector: ├── for rows 0-3, └── for last row
				if row < 4 {
					b.WriteString(treeStyle.Render("   ├ "))
				} else {
					b.WriteString(treeStyle.Render("   └ "))
				}
				b.WriteString(left)
				if right != "" {
					b.WriteString(right)
				}
				b.WriteByte('\n')
			}
		}
	}

	// ── Viewport (bordered scrollable log) ─────────────
	vpContent := m.applyGradient(m.viewport.View())
	bordered := viewportBorderStyle.Width(m.width).Render(vpContent)
	b.WriteString(bordered)
	b.WriteByte('\n')

	// ── Footer bar ─────────────────────────────────────
	b.WriteString(m.renderFooter())

	// Pad every line to full width and fill remaining height so the
	// entire alt screen is covered by colorBg with no terminal bg gaps.
	v.Content = m.fillScreen(b.String())
	return v
}

// fillScreen pads every line to m.width and adds blank bg-filled lines
// to reach m.height, ensuring the entire terminal is covered.
func (m buildModel) fillScreen(content string) string {
	bgFill := chromeBg.Render
	lines := strings.Split(content, "\n")

	var out strings.Builder
	for i, line := range lines {
		visible := lipgloss.Width(line)
		if visible < m.width {
			line += bgFill(strings.Repeat(" ", m.width-visible))
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}

	// Fill remaining vertical space
	blankLine := bgFill(strings.Repeat(" ", m.width))
	for i := len(lines); i < m.height; i++ {
		out.WriteByte('\n')
		out.WriteString(blankLine)
	}

	return out.String()
}

// renderStageCell renders a single stage line with icon, name, and elapsed time.
func (m buildModel) renderStageCell(index int) string {
	s := m.stages[index]

	var icon, name, status string
	switch s.status {
	case phaseRunning:
		icon = m.spinner.View()
		name = stageRunStyle.Render(s.name)
		elapsed := time.Since(s.start)
		status = chromeDim.Render(formatDuration(elapsed))
	case phaseComplete:
		icon = chromeSuccess.Render("✓")
		name = chromeText.Render(s.name)
		status = chromeDim.Render(formatDuration(s.elapsed))
	default:
		icon = chromeDim.Render("○")
		name = chromeDim.Render(s.name)
	}

	cell := chromeBg.Render("  ") + icon + chromeBg.Render(" ") + name
	if status != "" {
		cell += chromeBg.Render("  ") + status
	}
	return cell
}

// renderFooter builds a full-width status bar with spinner, progress, and scroll info.
// Every character has an explicit background so no terminal bg leaks through.
func (m buildModel) renderFooter() string {
	fSp := lipgloss.NewStyle().Background(colorBgDark) // footer spacer
	elapsed := formatDuration(time.Since(m.totalStart))

	// Left side: spinner label or phase progress
	var left string
	if m.active {
		spinnerView := lipgloss.NewStyle().Foreground(colorAccent).Background(colorBgDark).Render(m.spinner.View())
		spinElapsed := footerDimStyle.Render(formatDuration(time.Since(m.start)))
		left = fSp.Render(" ") + spinnerView + fSp.Render(" ") +
			footerBarStyle.Render(m.label) + fSp.Render("  ") + spinElapsed
	} else {
		completed := 0
		for _, p := range m.phases {
			if p.status == phaseComplete || p.status == phaseCached {
				completed++
			}
		}
		left = fSp.Render(" ") + renderProgressBar(completed, 9, 20) +
			footerBarStyle.Render(fmt.Sprintf(" %d/9", completed))
	}

	// Right side: scroll indicator + elapsed
	var right string
	if !m.autoScroll {
		pct := m.viewport.ScrollPercent()
		right = footerScrollStyle.Render(fmt.Sprintf("▲ %.0f%%", pct*100)) +
			footerDimStyle.Render("  ")
	}
	right += footerDimStyle.Render(elapsed) + fSp.Render(" ")

	// Pad middle with explicit bg
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := max(m.width-leftW-rightW, 0)

	return left + fSp.Render(strings.Repeat(" ", gap)) + right
}

// printSummary prints a detailed post-exit summary to stderr after the alt
// screen closes. Shows each stage with elapsed time, and under Build shows
// every phase with its status (cached, elapsed, or failed).
func (m buildModel) printSummary(err error) {
	totalElapsed := time.Since(m.totalStart)
	dim := summaryDim
	w := os.Stderr

	fmt.Fprintf(w, "\n %s %s %s\n\n",
		headerStyle.Render("starforge"),
		m.commandName,
		m.targetName)

	for i := 0; i < m.stageCount; i++ {
		s := m.stages[i]
		switch s.status {
		case phaseComplete:
			fmt.Fprintf(w, "  %s %s  %s\n",
				successStyle.Render("✓"),
				s.name,
				dim.Render(formatDuration(s.elapsed)))
		case phaseRunning:
			elapsed := time.Since(s.start)
			fmt.Fprintf(w, "  %s %s  %s\n",
				failStyle.Render("✗"),
				failStyle.Render(s.name),
				dim.Render(formatDuration(elapsed)))
		default:
			fmt.Fprintf(w, "  %s %s\n",
				dim.Render("○"),
				dim.Render(s.name))
		}

		// Show phase details under Build stage
		if Stage(i) == StageBuild {
			for j := range m.phases {
				p := m.phases[j]
				connector := "├"
				if j == len(m.phases)-1 {
					connector = "└"
				}
				switch p.status {
				case phaseCached:
					fmt.Fprintf(w, "    %s %s %s  %s\n",
						dim.Render(connector),
						successStyle.Render("✓"),
						p.name,
						cachedStyle.Render("cached"))
				case phaseComplete:
					fmt.Fprintf(w, "    %s %s %s  %s\n",
						dim.Render(connector),
						successStyle.Render("✓"),
						p.name,
						dim.Render(formatDuration(p.elapsed)))
				case phaseFailed:
					fmt.Fprintf(w, "    %s %s %s\n",
						dim.Render(connector),
						failStyle.Render("✗"),
						failStyle.Render(p.name))
				default:
					fmt.Fprintf(w, "    %s %s %s\n",
						dim.Render(connector),
						dim.Render("○"),
						dim.Render(p.name))
				}
			}
		}
	}

	fmt.Fprintln(w)
	if err != nil {
		fmt.Fprintf(w, "  %s  %s\n\n",
			failStyle.Render("failed"),
			dim.Render(formatDuration(totalElapsed)))
	} else {
		fmt.Fprintf(w, "  %s  %s\n\n",
			successStyle.Render("done"),
			dim.Render(formatDuration(totalElapsed)))
	}
}

// renderPhaseCell renders a single phase cell for the dashboard grid.
func (m buildModel) renderPhaseCell(index, width int) string {
	p := m.phases[index]

	var icon, name, status string
	switch p.status {
	case phasePending:
		icon = chromeDim.Render("○")
		name = chromeDim.Render(p.name)
	case phaseRunning:
		icon = m.spinner.View()
		name = chromeEmphasis.Render(p.name)
		elapsed := time.Since(p.start)
		status = chromeDim.Render(formatDuration(elapsed))
	case phaseCached:
		icon = chromeSuccess.Render("✓")
		name = chromeText.Render(p.name)
		status = chromeCached.Render("cached")
	case phaseComplete:
		icon = chromeSuccess.Render("✓")
		name = chromeText.Render(p.name)
		status = chromeDim.Render(formatDuration(p.elapsed))
	case phaseFailed:
		icon = chromeError.Render("✗")
		name = chromeError.Render(p.name)
	}

	cell := icon + chromeBg.Render(" ") + name
	if status != "" {
		cell += chromeBg.Render("  ") + status
	}

	visible := lipgloss.Width(cell)
	if visible < width {
		cell += chromeBg.Render(strings.Repeat(" ", width-visible))
	}
	return cell
}

// renderProgressBar draws a simple block progress bar.
func renderProgressBar(done, total, barWidth int) string {
	filled := 0
	if total > 0 {
		filled = done * barWidth / total
	}
	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", barWidth-filled)
	return footerDimStyle.Render("[") + footerAccentStyle.Render(filledStr) +
		footerDimStyle.Render(emptyStr) + footerDimStyle.Render("]")
}

// --- Writers ---

// lineWriter buffers input and prints complete lines through the output system.
type lineWriter struct {
	output *Output
	prefix string
	buf    bytes.Buffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf.Write(p)

	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Incomplete line — put it back
			w.buf.WriteString(line)
			break
		}
		line = strings.TrimRight(line, "\n\r")
		if line != "" {
			w.output.println(w.prefix+line, w.prefix+line)
		}
	}
	return n, nil
}

// logOnlyWriter writes to the log file only, suppressing terminal output.
type logOnlyWriter struct {
	output *Output
	buf    bytes.Buffer
}

func (w *logOnlyWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf.Write(p)

	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			w.buf.WriteString(line)
			break
		}
		line = strings.TrimRight(line, "\n\r")
		if line != "" {
			w.output.logLine("      " + line)
		}
	}
	return n, nil
}

// --- Gradient ---

// Gradient endpoints for the log viewport background.
// Top is slightly brighter, bottom fades to a deeper dark.
var (
	gradientTop    = color.RGBA{R: 0x28, G: 0x1e, B: 0x10, A: 0xff} // warm dark, slightly lighter than bg
	gradientBottom = color.RGBA{R: 0x12, G: 0x0d, B: 0x08, A: 0xff} // matches colorBgDark
)

// lerpColor linearly interpolates between two colors. t in [0,1].
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8, t float64) uint8 {
		return uint8(float64(x) + t*(float64(y)-float64(x)))
	}
	return color.RGBA{
		R: lerp(a.R, b.R, t),
		G: lerp(a.G, b.G, t),
		B: lerp(a.B, b.B, t),
		A: 0xff,
	}
}

// applyGradient injects a per-row background color into each visible line
// of the viewport output using raw ANSI 24-bit color escapes. Lipgloss
// Render() inserts \033[0m resets that would clear any outer background,
// so we replace every reset with reset+re-apply to keep the gradient
// persistent across the entire line.
func (m buildModel) applyGradient(vpOutput string) string {
	lines := strings.Split(vpOutput, "\n")
	h := len(lines)
	if h == 0 {
		return vpOutput
	}
	const (
		ansiReset  = "\x1b[m"  // lipgloss v2 uses \x1b[m (not \x1b[0m)
		ansiReset0 = "\x1b[0m" // some tools use the explicit form
		ansiBgFmt  = "\x1b[48;2;%d;%d;%dm"
	)
	vpW := m.viewport.Width()
	var b strings.Builder
	for i, line := range lines {
		t := float64(i) / float64(max(h-1, 1))
		c := lerpColor(gradientTop, gradientBottom, t)
		bgSeq := fmt.Sprintf(ansiBgFmt, c.R, c.G, c.B)

		// Inject background: prepend bg, replace every reset with
		// reset+bg so the color persists through inner styled spans.
		line = strings.ReplaceAll(line, ansiReset0, ansiReset0+bgSeq)
		line = strings.ReplaceAll(line, ansiReset, ansiReset+bgSeq)
		line = bgSeq + line

		// Ensure the line fills the full viewport width.
		visible := lipgloss.Width(line)
		if visible < vpW {
			line += strings.Repeat(" ", vpW-visible)
		}

		b.WriteString(line)
		b.WriteString(ansiReset)
		if i < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// --- Utilities ---

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

// IsInteractive returns true if stderr is a terminal.
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}
