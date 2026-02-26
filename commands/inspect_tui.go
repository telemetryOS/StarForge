package commands

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/telemetryos/starforge/actions"
)

const sidebarWidth = 20

// contentPad is applied to the left edge of the content panel.
var contentPad = lipgloss.NewStyle().PaddingLeft(1).Background(tuiBg)

type inspectModel struct {
	ctx         *actions.BuildContext
	target      string
	sections    []section
	cursor      int      // selected sidebar section
	prevCursor  int      // tracks section changes for GotoTop
	showLayers  bool
	searching   bool
	searchInput textinput.Model
	viewport    viewport.Model
	width       int
	height      int
	ready       bool
	cache       map[string]string
	cacheLayer  bool
}

func newInspectModel(target string, ctx *actions.BuildContext, sections []section, cursor int) inspectModel {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.Prompt = "/ "
	s := ti.Styles()
	s.Focused.Prompt = searchPromptStyle
	ti.SetStyles(s)
	ti.CharLimit = 80

	return inspectModel{
		ctx:        ctx,
		target:     target,
		sections:   sections,
		cursor:     cursor,
		prevCursor: -1, // force initial content load
		searchInput: ti,
		cache:      make(map[string]string),
	}
}

func (m inspectModel) Init() tea.Cmd {
	return nil
}

func (m inspectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateNormal(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		wasReady := m.ready
		if !m.ready {
			m.ready = true
		}
		m.resizeViewport()
		if !wasReady {
			m.loadSection()
		}
		return m, nil
	}

	// Pass mouse events etc. through to viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m inspectModel) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit

	// Sidebar navigation
	case "tab", "shift+tab":
		if msg.String() == "tab" {
			m.cursor = (m.cursor + 1) % len(m.sections)
		} else {
			m.cursor = (m.cursor - 1 + len(m.sections)) % len(m.sections)
		}
		m.loadSection()
		return m, nil

	// Content scrolling
	case "up", "k":
		m.viewport.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.viewport.ScrollDown(1)
		return m, nil
	case "pgup", "ctrl+u":
		m.viewport.HalfPageUp()
		return m, nil
	case "pgdown", "ctrl+d":
		m.viewport.HalfPageDown()
		return m, nil
	case "g":
		m.viewport.GotoTop()
		return m, nil
	case "G":
		m.viewport.GotoBottom()
		return m, nil

	// Features
	case "l":
		m.showLayers = !m.showLayers
		m.invalidateCache()
		m.loadSection()
		return m, nil
	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	// Number keys for direct section jump
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0]-'0') - 1
		if idx < len(m.sections) {
			m.cursor = idx
			m.loadSection()
		}
		return m, nil
	case "0":
		if 9 < len(m.sections) {
			m.cursor = 9
			m.loadSection()
		}
		return m, nil
	}

	// Unknown keys fall through to viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m inspectModel) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchInput.Reset()
		m.searchInput.Blur()
		m.loadSection() // re-render without highlights
		return m, nil
	case "enter":
		m.scrollToMatch()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	// Update highlights as the user types
	m.setViewportContent()
	return m, cmd
}

// invalidateCache clears the rendered content cache.
func (m *inspectModel) invalidateCache() {
	m.cache = make(map[string]string)
}

// renderSection returns cached rendered content for a section key.
func (m *inspectModel) renderSection(key string) string {
	if m.cacheLayer != m.showLayers {
		m.invalidateCache()
		m.cacheLayer = m.showLayers
	}
	if content, ok := m.cache[key]; ok {
		return content
	}

	inspectLayers = m.showLayers

	var w strings.Builder
	renderConcern(&w, key, m.ctx)
	content := w.String()
	m.cache[key] = content
	return content
}

// resizeViewport updates viewport dimensions without changing content.
func (m *inspectModel) resizeViewport() {
	if !m.ready {
		return
	}
	contentWidth := max(m.width-sidebarWidth-3, 20)
	contentHeight := max(m.height-2, 1) // title + footer
	m.viewport.SetWidth(contentWidth)
	m.viewport.SetHeight(contentHeight)
}

// loadSection sets the viewport content for the current cursor position.
// Scrolls to top only when the section actually changed.
func (m *inspectModel) loadSection() {
	if !m.ready {
		return
	}
	sectionChanged := m.cursor != m.prevCursor
	m.prevCursor = m.cursor

	m.setViewportContent()

	if sectionChanged {
		m.viewport.GotoTop()
	}
}

// setViewportContent renders the current section into the viewport,
// applying search highlights if active. Does not change scroll position.
func (m *inspectModel) setViewportContent() {
	key := m.sections[m.cursor].key
	content := m.renderSection(key)

	if m.searching && m.searchInput.Value() != "" {
		content = m.highlightMatches(content)
	}

	m.viewport.SetContent(content)
}

// highlightMatches highlights search terms in the content.
func (m *inspectModel) highlightMatches(content string) string {
	query := m.searchInput.Value()
	if query == "" {
		return content
	}
	lower := strings.ToLower(query)
	var result strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(strings.ToLower(line), lower) {
			lineLower := strings.ToLower(line)
			idx := strings.Index(lineLower, lower)
			for idx >= 0 {
				result.WriteString(line[:idx])
				result.WriteString(searchMatchStyle.Render(line[idx : idx+len(query)]))
				line = line[idx+len(query):]
				lineLower = strings.ToLower(line)
				idx = strings.Index(lineLower, lower)
			}
			result.WriteString(line)
		} else {
			result.WriteString(line)
		}
		result.WriteByte('\n')
	}
	return result.String()
}

// scrollToMatch scrolls to the next match after the current scroll position.
func (m *inspectModel) scrollToMatch() {
	query := m.searchInput.Value()
	if query == "" {
		return
	}
	lower := strings.ToLower(query)
	key := m.sections[m.cursor].key
	content := m.renderSection(key)
	lines := strings.Split(content, "\n")
	start := m.viewport.YOffset() + 1
	for i := range len(lines) {
		lineIdx := (start + i) % len(lines)
		if strings.Contains(strings.ToLower(lines[lineIdx]), lower) {
			m.viewport.SetYOffset(lineIdx)
			return
		}
	}
}

func (m inspectModel) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	if !m.ready {
		v.Content = "\n  Initializing..."
		return v
	}

	var b strings.Builder

	// Title bar
	b.WriteString(tuiTitleBar("inspect", m.target, m.width))
	b.WriteByte('\n')

	// Main area: sidebar | content
	mainHeight := max(m.height-2, 1) // title + footer

	sidebar := m.renderSidebar()
	styledSidebar := sidebarBorder.
		Width(sidebarWidth).
		Height(mainHeight).
		Render(sidebar)

	content := m.viewport.View()
	styledContent := contentPad.
		Width(m.viewport.Width() + 1).
		Height(mainHeight).
		Render(content)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, styledSidebar, styledContent))
	b.WriteByte('\n')

	// Footer bar
	b.WriteString(m.renderHelpBar())

	v.Content = tuiFillScreen(b.String(), m.width, m.height)
	return v
}

func (m inspectModel) renderSidebar() string {
	var b strings.Builder
	for i, sec := range m.sections {
		prefix := "  "
		countStr := ""
		if sec.count >= 0 {
			countStr = " " + sidebarCount.Render(fmt.Sprintf("(%d)", sec.count))
		}

		var line string
		if i == m.cursor {
			prefix = "> "
			line = prefix + sidebarSelected.Render(sec.name) + countStr
		} else if sec.empty {
			line = prefix + sidebarEmpty.Render(sec.name) + countStr
		} else {
			line = prefix + sidebarNormal.Render(sec.name) + countStr
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Layer toggle indicator at bottom
	layerHeight := m.height - 1 - len(m.sections) - 2
	if layerHeight > 0 {
		b.WriteString(strings.Repeat("\n", layerHeight))
	}
	if m.showLayers {
		b.WriteString(" " + sidebarSelected.Render("layers: on"))
	} else {
		b.WriteString(" " + sidebarEmpty.Render("layers: off"))
	}

	return b.String()
}

func (m inspectModel) renderHelpBar() string {
	if m.searching {
		return tuiFooterBar(
			tuiFooterDim.Render(" ")+m.searchInput.View(),
			"",
			m.width,
		)
	}

	left := tuiFooterDim.Render(" ↑/↓ scroll  tab section  l layers  / search  q quit")

	var right string
	if m.viewport.TotalLineCount() > 0 {
		right = tuiFooterAccent.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)) +
			tuiFooterDim.Render(" ")
	}

	return tuiFooterBar(left, right, m.width)
}

func runInspectTUI(target string, ctx *actions.BuildContext, cursor int) error {
	sections := buildSections(ctx)
	model := newInspectModel(target, ctx, sections, cursor)

	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
