package main

import (
	"fmt"
	"strings"

	"argo/src/brig"
	"argo/src/knot"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ---- 自定义消息类型 ----

type sseEventMsg struct{ ev knot.Event }
type streamCompleteMsg struct{}
type streamErrorMsg struct{ err error }
type streamStartedMsg struct {
	events <-chan knot.Event
	cancel func()
}

// ---- Model ----

type model struct {
	viewport viewport.Model
	textarea textarea.Model

	client    *argoClient
	sessionID string

	output    strings.Builder
	statusBar string

	streaming  bool
	streamCh   <-chan knot.Event
	cancelFunc func()
	seenTools  map[string]bool

	asking         bool
	askingTUs      []knot.ToolUse
	askIndex       int
	askInterrupted bool

	width  int
	height int
}

// ---- 构造函数 ----

func initialModel(client *argoClient, sessionID string) *model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 15
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))

	// 设置 textarea 整体背景色（color 60），消除黑色默认背景
	bg := lipgloss.NewStyle().Background(lipgloss.Color("60"))
	st := textarea.DefaultStyles(true)
	st.Focused.Base = bg
	st.Focused.CursorLine = bg
	st.Focused.EndOfBuffer = bg
	st.Focused.Text = bg
	st.Blurred.Base = bg
	st.Blurred.CursorLine = bg
	st.Blurred.EndOfBuffer = bg
	st.Blurred.Text = bg
	ta.SetStyles(st)
	ta.Focus()

	vp := viewport.New()

	return &model{
		viewport:  vp,
		textarea:  ta,
		client:    client,
		sessionID: sessionID,
		seenTools: make(map[string]bool),
		statusBar: "Ready",
	}
}

// ---- Init ----

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
	)
}

// ---- Update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetWidth(msg.Width)
		m.recalcViewport()
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case sseEventMsg:
		return m.handleSSEEvent(msg.ev)

	case streamStartedMsg:
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		m.streamCh = msg.events
		m.cancelFunc = msg.cancel
		m.streaming = true
		return m, waitForSSEEvent(m.streamCh)

	case streamCompleteMsg:
		m.streaming = false
		return m, nil

	case streamErrorMsg:
		m.streaming = false
		m.output.WriteString("\n" + errorStyle.Render("Error: "+msg.err.Error()) + "\n")
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// recalcViewport 根据 textarea 实际高度调整 viewport 高度
func (m *model) recalcViewport() {
	if m.height == 0 {
		return
	}
	taHeight := m.textarea.Height()
	m.viewport.SetHeight(m.height - taHeight - 1) // -1 状态行
}

// handleKeyPress 处理键盘输入
func (m *model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.streaming {
			m.cancelFunc()
			m.client.interruptSession(m.sessionID)
			m.output.WriteString("\n" + errorStyle.Render("... Interrupted ...") + "\n")
			m.streaming = false
			m.viewport.SetContent(m.output.String())
			m.viewport.GotoBottom()
			return m, nil
		}
		return m, nil

		case "ctrl+c":
			return m, tea.Quit

		case "enter":
		if m.asking {
			return m.handleAskVerdict()
		}
		return m.handleSubmit()
	}

	if m.asking {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		m.recalcViewport()
		return m, cmd
	}

	// pgup/pgdown 滚 viewport，↑ ↓ 给 textarea 移光标
	switch msg.String() {
	case "pgup", "pgdown":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case "up", "down":
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		m.recalcViewport()
		return m, cmd
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.recalcViewport()
	return m, cmd
}

// handleSubmit Enter 提交
func (m *model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}
	if input == "/exit" {
		return m, tea.Quit
	}
	m.textarea.Reset()
	m.recalcViewport()

	sep := strings.Repeat("─", m.width)
	m.output.WriteString("\n\n" + sep + "\n" + userMsgStyle.Render("❯ "+input) + "\n" + sep + "\n")
	m.viewport.SetContent(m.output.String())
	m.viewport.GotoBottom()
	return m, startSSEStreamCmd(m.client, m.sessionID, input)
}

// ---- View ----

func (m *model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Initializing...")
	}
	vpView := m.viewport.View()
	sbView := statusBarStyle.Width(m.width).Render(m.statusBar)
	taView := padLines(m.textarea.View(), m.width)

	rendered := lipgloss.JoinVertical(lipgloss.Top, vpView, sbView, taView)
	return tea.NewView(rendered)
}

// ---- SSE 桥接 Commands ----

func waitForSSEEvent(events <-chan knot.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return streamCompleteMsg{}
		}
		return sseEventMsg{ev: ev}
	}
}

func startSSEStreamCmd(c *argoClient, sessionID, prompt string) tea.Cmd {
	return func() tea.Msg {
		events, cancel, err := c.sendPrompt(sessionID, prompt)
		if err != nil {
			return streamErrorMsg{err: err}
		}
		return streamStartedMsg{events: events, cancel: cancel}
	}
}

func restartStreamWithAskReply(c *argoClient, sessionID string, tus []knot.ToolUse) tea.Cmd {
	return func() tea.Msg {
		events, cancel, err := c.sendAskReply(sessionID, tus)
		if err != nil {
			return streamErrorMsg{err: err}
		}
		return streamStartedMsg{events: events, cancel: cancel}
	}
}

// ---- SSE 事件处理 ----

func (m *model) handleSSEEvent(ev knot.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {

	case knot.EventTextDelta:
		m.output.WriteString(ev.Delta)
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventTextDone:
		if ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0 {
			m.statusBar = fmt.Sprintf("tokens: in %d | out %d",
				ev.Usage.InputTokens, ev.Usage.OutputTokens)
		}

	case knot.EventThinkingStart:
		m.output.WriteString("\n" + thinkingStyle.Render("── Thinking ──") + "\n")
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventThinkingDelta:
		m.output.WriteString(thinkingStyle.Render(ev.Delta))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventThinkingDone:
		m.output.WriteString("\n" + thinkingStyle.Render("── Thinking Complete ──") + "\n")
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventToolUseStart:
		if ev.ToolUse.ID != "" && !m.seenTools[ev.ToolUse.ID] {
			m.seenTools[ev.ToolUse.ID] = true
			m.output.WriteString(formatToolStart(ev))
			m.viewport.SetContent(m.output.String())
			m.viewport.GotoBottom()
		}

	case knot.EventToolUseDone:
		delete(m.seenTools, ev.ToolUse.ID)
		m.output.WriteString(formatToolDone(ev))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventAsk:
		m.asking = true
		m.askingTUs = ev.AskingToolUses
		m.askIndex = 0
		m.askInterrupted = false
		m.textarea.Placeholder = "Ask [y/N/stop]: "
		m.textarea.Focus()
		tu := m.askingTUs[0]
		m.output.WriteString(formatAskPrompt(tu))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventError:
		clear(m.seenTools)
		m.output.WriteString(errorStyle.Render(fmt.Sprintf("\nError: %v\n", ev.Err)))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventDone:
		clear(m.seenTools)
	}

	return m, waitForSSEEvent(m.streamCh)
}

// ---- Ask 裁决处理 ----

func (m *model) handleAskVerdict() (tea.Model, tea.Cmd) {
	response := strings.TrimSpace(m.textarea.Value())
	m.textarea.Reset()
	m.recalcViewport()

	tu := m.askingTUs[m.askIndex]

	switch strings.ToLower(response) {
	case "stop", "/interrupt":
		m.askInterrupted = true
		m.cancelFunc()
		m.client.interruptSession(m.sessionID)
		m.output.WriteString("\n" + errorStyle.Render("Session interrupted.") + "\n")
		m.asking = false
		m.streaming = false
		m.textarea.Placeholder = ""
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, nil

	case "y":
		tu.VerdictResult = brig.UserApprove
	default:
		tu.VerdictResult = brig.UserDeny
	}

	m.askingTUs[m.askIndex] = tu
	m.askIndex++

	if m.askIndex >= len(m.askingTUs) {
		m.asking = false
		m.textarea.Placeholder = ""
		clear(m.seenTools)
		m.cancelFunc()
		return m, restartStreamWithAskReply(m.client, m.sessionID, m.askingTUs)
	}

	nextTU := m.askingTUs[m.askIndex]
	m.output.WriteString(formatAskPrompt(nextTU))
	m.viewport.SetContent(m.output.String())
	m.viewport.GotoBottom()
	return m, nil
}

// ---- 辅助 ----

func padLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < width {
			lines[i] = line + strings.Repeat(" ", width-w)
		}
	}
	return strings.Join(lines, "\n")
}
