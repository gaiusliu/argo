package main

import (
	"fmt"
	"strings"

	"argo/src/brig"
	"argo/src/knot"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- 自定义消息类型 ----

// sseEventMsg SSE channel 中的 knot.Event 包装
type sseEventMsg struct{ ev knot.Event }

// streamCompleteMsg SSE channel 关闭时发送
type streamCompleteMsg struct{}

// streamErrorMsg SSE 流启动失败时发送
type streamErrorMsg struct{ err error }

// streamStartedMsg 新 SSE 流启动后携带 channel + cancel func
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
	ta.Placeholder = "argo> "
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(80, 20)

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
		// 计算布局：viewport 高度 = 终端高度 - 状态行(1) - textarea(3)
		inputHeight := lipgloss.Height(m.textarea.View())
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - inputHeight - 1
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case sseEventMsg:
		return m.handleSSEEvent(msg.ev)

	case streamStartedMsg:
		// 取消旧流（如有），存储新流
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

	// 默认：把消息传给 textarea
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// handleKeyPress 处理键盘输入，按 asking 模式分流
func (m *model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m, tea.Quit

	case "enter":
		if m.asking {
			return m.handleAskVerdict()
		}
		return m.handleSubmit()
	}

	// 非特殊键 → 传给 textarea 或 viewport
	if m.asking {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	// 正常模式：↑ ↓ pgup pgdown 滚 viewport
	switch msg.String() {
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// handleSubmit 正常模式下的 Enter：发送 prompt
func (m *model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}
	if input == "/exit" {
		return m, tea.Quit
	}
	m.textarea.Reset()
	m.output.WriteString("argo> " + input + "\n")
	m.viewport.SetContent(m.output.String())
	m.viewport.GotoBottom()
	return m, startSSEStreamCmd(m.client, m.sessionID, input)
}

// ---- View ----

func (m *model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}
	vpView := m.viewport.View()
	sbView := statusBarStyle.Width(m.width).Render(m.statusBar)
	taView := inputStyle.Render(m.textarea.View())

	return lipgloss.JoinVertical(lipgloss.Top, vpView, sbView, taView)
}

// ---- SSE 桥接 Commands ----

// waitForSSEEvent 阻塞等待下一个 SSE 事件，包装为 tea.Msg
func waitForSSEEvent(events <-chan knot.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return streamCompleteMsg{}
		}
		return sseEventMsg{ev: ev}
	}
}

// startSSEStreamCmd 发起 prompt，返回 streamStartedMsg
func startSSEStreamCmd(c *argoClient, sessionID, prompt string) tea.Cmd {
	return func() tea.Msg {
		events, cancel, err := c.sendPrompt(sessionID, prompt)
		if err != nil {
			return streamErrorMsg{err: err}
		}
		return streamStartedMsg{events: events, cancel: cancel}
	}
}

// restartStreamWithAskReply 取消旧流，发送 Ask 裁决，启动新流
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

// handleSSEEvent 分发 knot.Event 到各 EventType 处理
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
		m.output.WriteString(ev.Delta)
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
		// 首个 Ask 工具提示追加到 output
		tu := m.askingTUs[0]
		m.output.WriteString(formatAskPrompt(tu))
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventError:
		clear(m.seenTools)
		errMsg := errorStyle.Render(fmt.Sprintf("\nError: %v\n", ev.Err))
		m.output.WriteString(errMsg)
		m.viewport.SetContent(m.output.String())
		m.viewport.GotoBottom()

	case knot.EventDone:
		clear(m.seenTools)
	}

	// 继续等待下一个 SSE 事件
	return m, waitForSSEEvent(m.streamCh)
}

// ---- Ask 裁决处理 ----

// handleAskVerdict 处理 Ask 模式下的用户输入
func (m *model) handleAskVerdict() (tea.Model, tea.Cmd) {
	response := strings.TrimSpace(m.textarea.Value())
	m.textarea.Reset()

	tu := m.askingTUs[m.askIndex]

	switch strings.ToLower(response) {
	case "stop", "/interrupt":
		m.askInterrupted = true
		m.cancelFunc()
		m.client.interruptSession(m.sessionID)
		m.output.WriteString("\n" + errorStyle.Render("Session interrupted.") + "\n")
		m.asking = false
		m.streaming = false
		m.textarea.Placeholder = "argo> "
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
		// 全部裁决完成 → 发送 Ask 回复
		m.asking = false
		m.textarea.Placeholder = "argo> "
		clear(m.seenTools)
		m.cancelFunc()
		return m, restartStreamWithAskReply(m.client, m.sessionID, m.askingTUs)
	}

	// 展示下一个工具的 Ask 提示
	nextTU := m.askingTUs[m.askIndex]
	m.output.WriteString(formatAskPrompt(nextTU))
	m.viewport.SetContent(m.output.String())
	m.viewport.GotoBottom()
	return m, nil
}
