package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/tools"
)

type tuiState int

const (
	stateIdle      tuiState = iota
	stateThinking           // waiting for model
	stateApproval           // waiting for bash approval
	stateAskPicker          // waiting for ask answer
)

// Model is the bubbletea model for the agent TUI.
type Model struct {
	// Session config
	providerName  string
	provider      ai.AgentProvider
	modelID       string
	system        string
	contextWindow int

	// Allow-list
	cfgPath    string
	allowRules []string // user-configured rules from config.Shell.Allow

	// Conversation
	messages []ai.Message
	thread   []ThreadEntry
	stdin    string // pre-loaded stdin, attached to first message

	// State
	state tuiState

	// Pending tool loop (used during stateApproval / stateAskPicker)
	pendingToolCall  ai.ToolCall
	pendingRemaining []ai.ToolCall
	pendingCollected []ai.ToolResult

	// Ask picker
	activePicker *picker

	// Token counters
	inputTokens  int
	outputTokens int

	// Components
	vp    viewport.Model
	input textinput.Model
	spin  spinner.Model

	// Layout
	width  int
	height int
	ready  bool

	// Cancellation (ctrl+c / esc during thinking)
	ctx    context.Context
	cancel context.CancelFunc

	// Double ctrl+c tracking
	lastCtrlC time.Time

	// Styles (built once)
	styles       Styles
	glamourStyle string // "dark" or "light", fixed at construction
}

// newModel constructs the TUI model.
func newModel(
	providerName string,
	provider ai.AgentProvider,
	modelID string,
	system string,
	stdin string,
	r *lipgloss.Renderer,
	cfgPath string,
	allowRules []string,
	prefill string,
) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.SetValue(prefill)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	contextWindow := 32_000
	if md := models.Find(providerName, modelID); md != nil && md.Context > 0 {
		contextWindow = md.Context
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := Model{
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		system:        system,
		stdin:         stdin,
		contextWindow: contextWindow,
		cfgPath:       cfgPath,
		allowRules:    allowRules,
		input:         ti,
		spin:          sp,
		ctx:           ctx,
		cancel:        cancel,
		styles:        newStyles(r),
		glamourStyle:  glamourStyle(r),
	}

	if stdin != "" {
		m.thread = append(m.thread, ThreadEntry{
			Kind:    EntryUser,
			Content: fmt.Sprintf("[stdin: %d lines]", strings.Count(stdin, "\n")+1),
		})
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, textinput.Blink)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Window resize ──────────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpW, vpH := m.viewportDims()
		if !m.ready {
			m.vp = viewport.New(vpW, vpH)
			m.ready = true
		} else {
			m.vp.Width = vpW
			m.vp.Height = vpH
		}
		m.input.Width = vpW - 4
		m.refreshViewport()
		return m, nil

	// ── Spinner tick ───────────────────────────────────────────────────────────
	case spinner.TickMsg:
		if m.state == stateThinking {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	// ── Agent: model responded ─────────────────────────────────────────────────
	case agent.ResponseMsg:
		if msg.Err != nil {
			m.state = stateIdle
			m.thread = append(m.thread, ThreadEntry{Kind: EntryError, Content: msg.Err.Error()})
			m.refreshViewport()
			return m, nil
		}

		resp := msg.Resp
		m.inputTokens += resp.InputTokens
		m.outputTokens += resp.OutputTokens

		// Append assistant message to history.
		assistantMsg := ai.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		m.messages = append(m.messages, assistantMsg)

		// Show bare text in thread (if any, and no tool calls — implicit respond).
		if resp.Content != "" && len(resp.ToolCalls) == 0 {
			m.thread = append(m.thread, ThreadEntry{Kind: EntryAssistant, Content: resp.Content})
			m.refreshViewport()
			m.state = stateIdle
			return m, nil
		}

		if len(resp.ToolCalls) == 0 {
			m.state = stateIdle
			m.refreshViewport()
			return m, nil
		}

		// Start processing tool calls.
		return m, agent.ProcessToolsCmd(resp.ToolCalls, nil, m.needsApproval)

	// ── Agent: tool executed (read or auto-bash) ───────────────────────────────
	case agent.ToolExecutedMsg:
		tc := msg.ToolCall
		m.thread = append(m.thread,
			ThreadEntry{
				Kind:       EntryToolCall,
				ToolName:   tc.Name,
				ToolDetail: toolDetail(tc),
				Auto:       msg.AutoAccepted,
			},
			ThreadEntry{
				Kind:    EntryToolResult,
				Content: msg.Result.Content,
				IsError: msg.Result.IsError,
			},
		)
		m.refreshViewport()

		if len(msg.Remaining) == 0 {
			return m, agent.ProcessToolsCmd(nil, msg.Collected, m.needsApproval)
		}
		return m, agent.ProcessToolsCmd(msg.Remaining, msg.Collected, m.needsApproval)

	// ── Agent: bash needs approval ─────────────────────────────────────────────
	case agent.NeedsApprovalMsg:
		m.state = stateApproval
		m.pendingToolCall = msg.ToolCall
		m.pendingRemaining = msg.Remaining
		m.pendingCollected = msg.Collected
		// Show the command in the thread (without result yet).
		m.thread = append(m.thread, ThreadEntry{
			Kind:       EntryToolCall,
			ToolName:   msg.ToolCall.Name,
			ToolDetail: msg.Command,
			Auto:       false,
		})
		m.refreshViewport()
		return m, nil

	// ── Agent: ask tool ───────────────────────────────────────────────────────
	case agent.AskMsg:
		m.state = stateAskPicker
		m.pendingToolCall = msg.ToolCall
		m.pendingRemaining = msg.Remaining
		m.pendingCollected = msg.Collected
		p := newPicker(msg.Question, msg.Options, msg.MultiSelect)
		m.activePicker = &p
		// Show the question in the thread.
		if msg.Question != "" {
			m.thread = append(m.thread, ThreadEntry{Kind: EntryAssistant, Content: msg.Question})
		}
		m.refreshViewport()
		return m, nil

	// ── Agent: respond tool ───────────────────────────────────────────────────
	case agent.RespondMsg:
		m.thread = append(m.thread, ThreadEntry{
			Kind:        EntryRespond,
			RespondType: msg.RespondType,
			Content:     msg.Content,
		})
		// Append tool results as a user message, then return to idle.
		m.messages = append(m.messages, ai.Message{
			Role:        "user",
			ToolResults: msg.Collected,
		})
		m.refreshViewport()
		m.state = stateIdle
		return m, nil

	// ── Agent: all non-respond tools done — send results back to model ─────────
	case agent.AllToolsDoneMsg:
		m.messages = append(m.messages, ai.Message{
			Role:        "user",
			ToolResults: msg.Collected,
		})
		m.state = stateThinking
		return m, tea.Batch(
			m.spin.Tick,
			agent.ChatCmd(m.ctx, m.provider, m.chatRequest()),
		)

	// ── Keyboard ──────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to viewport when idle/thinking.
	if m.state == stateIdle || m.state == stateThinking {
		var vpCmd tea.Cmd
		m.vp, vpCmd = m.vp.Update(msg)
		return m, vpCmd
	}

	return m, nil
}

// handleKey handles all keyboard input based on current state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Double ctrl+c = quit regardless of state.
	if msg.Type == tea.KeyCtrlC {
		if time.Since(m.lastCtrlC) < time.Second {
			return m, tea.Quit
		}
		m.lastCtrlC = time.Now()
		if m.state == stateThinking {
			m.cancel()
			ctx, cancel := context.WithCancel(context.Background())
			m.ctx = ctx
			m.cancel = cancel
			m.state = stateIdle
			m.thread = append(m.thread, ThreadEntry{Kind: EntryError, Content: "interrupted"})
			m.refreshViewport()
		}
		return m, nil
	}

	switch m.state {
	case stateIdle:
		return m.handleIdleKey(msg)
	case stateApproval:
		return m.handleApprovalKey(msg)
	case stateAskPicker:
		return m.handlePickerKey(msg)
	}
	return m, nil
}

func (m Model) handleIdleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.input.SetValue("")
		return m.submitMessage(text)

	case tea.KeyEsc:
		// ESC while idle does nothing.
		return m, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Approve: execute the command.
		m.state = stateThinking
		tc := m.pendingToolCall
		remaining := m.pendingRemaining
		collected := m.pendingCollected
		return m, tea.Batch(
			m.spin.Tick,
			agent.ExecuteBashCmd(tc, remaining, collected),
		)

	case tea.KeyEsc:
		// Deny: add denial result and continue processing.
		denial := ai.ToolResult{
			ToolCallID: m.pendingToolCall.ID,
			Content:    "User denied this command.",
			IsError:    true,
		}
		m.thread = append(m.thread, ThreadEntry{
			Kind:    EntryToolResult,
			Content: "User denied this command.",
			IsError: true,
		})
		collected := append(m.pendingCollected, denial)
		remaining := m.pendingRemaining
		m.state = stateThinking
		m.refreshViewport()
		return m, tea.Batch(
			m.spin.Tick,
			agent.ProcessToolsCmd(remaining, collected, m.needsApproval),
		)

	case tea.KeyRunes:
		if msg.String() == "a" {
			// Allow + add to user allow-list: save rule and approve.
			cmd, _ := m.pendingToolCall.Input["command"].(string)
			rule := allowlist.BuildRuleFromCommand(cmd)
			if rule != "" {
				m.allowRules = append(m.allowRules, rule)
				m.saveAllowRules()
			}
			m.state = stateThinking
			tc := m.pendingToolCall
			remaining := m.pendingRemaining
			collected := m.pendingCollected
			return m, tea.Batch(
				m.spin.Tick,
				agent.ExecuteBashCmd(tc, remaining, collected),
			)
		}
	}
	return m, nil
}

// needsApproval returns true if the bash command must be confirmed by the user.
func (m Model) needsApproval(cmd string) bool {
	return allowlist.NeedsApproval(m.allowRules, cmd)
}

// saveAllowRules persists the current allowRules to config on disk.
// Failures are silently ignored — the in-memory list is still updated.
func (m Model) saveAllowRules() {
	if m.cfgPath == "" {
		return
	}
	cfg, _, err := config.LoadConfig(m.cfgPath)
	if err != nil {
		return
	}
	cfg.Shell.Allow = m.allowRules
	_ = config.SaveConfig(m.cfgPath, cfg)
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activePicker == nil {
		return m, nil
	}
	updated, result, submitted, cancelled := m.activePicker.Update(msg)
	m.activePicker = &updated

	if cancelled {
		result = "User cancelled"
		submitted = true
	}

	if submitted {
		m.activePicker = nil
		// Record answer in thread.
		m.thread = append(m.thread, ThreadEntry{Kind: EntryUser, Content: result})
		// Add tool result and continue.
		toolResult := ai.ToolResult{
			ToolCallID: m.pendingToolCall.ID,
			Content:    result,
		}
		collected := append(m.pendingCollected, toolResult)
		remaining := m.pendingRemaining
		m.state = stateThinking
		m.refreshViewport()
		return m, tea.Batch(
			m.spin.Tick,
			agent.ProcessToolsCmd(remaining, collected, m.needsApproval),
		)
	}
	return m, nil
}

// submitMessage adds the user's message and starts a model request.
func (m Model) submitMessage(text string) (tea.Model, tea.Cmd) {
	// Attach stdin to first user message if present.
	content := text
	if m.stdin != "" {
		content = text + "\n\n" + m.stdin
		m.stdin = ""
	}

	m.thread = append(m.thread, ThreadEntry{Kind: EntryUser, Content: text})
	m.messages = append(m.messages, ai.Message{Role: "user", Content: content})
	m.messages = trimContext(m.messages, m.contextWindow)
	m.state = stateThinking
	m.refreshViewport()

	return m, tea.Batch(
		m.spin.Tick,
		agent.ChatCmd(m.ctx, m.provider, m.chatRequest()),
	)
}

// chatRequest builds the ChatRequest from current state.
func (m Model) chatRequest() ai.ChatRequest {
	return ai.ChatRequest{
		Model:    m.modelID,
		System:   m.system,
		Messages: m.messages,
		Tools:    tools.Defs,
	}
}

// refreshViewport re-renders the thread and updates viewport content.
func (m *Model) refreshViewport() {
	content := renderThread(m.thread, m.vp.Width, m.styles, m.glamourStyle)
	m.vp.SetContent(content)
	m.vp.GotoBottom()
}

// popupHeight is the maximum number of terminal lines the TUI occupies.
// Inline mode (no alt screen) renders in place, so we cap the height to
// keep it feeling like a small popup rather than a full-page takeover.
const popupHeight = 20

// viewportDims calculates viewport dimensions from terminal size.
func (m Model) viewportDims() (width, height int) {
	// Layout: top border(1) | viewport | input(1) | bottom border(1)
	// Outer style: 1 border char each side = 2 chars total horizontal chrome.
	innerW := m.width - 2
	if innerW < 10 {
		innerW = 10
	}
	h := popupHeight
	if m.height > 0 && m.height < h {
		h = m.height
	}
	innerH := h - 3 // top border(1) + input(1) + bottom border(1)
	if innerH < 3 {
		innerH = 3
	}
	return innerW, innerH
}

// trimContext drops oldest tool results when approaching the context window limit.
func trimContext(messages []ai.Message, contextWindow int) []ai.Message {
	const (
		fixedTokens   = 700 // system prompt + tool defs estimate
		charsPerToken = 4
		threshold     = 0.70
	)

	limit := int(float64(contextWindow) * threshold)

	estimate := func(msgs []ai.Message) int {
		total := fixedTokens
		for _, m := range msgs {
			total += len(m.Content) / charsPerToken
			for _, tr := range m.ToolResults {
				total += len(tr.Content) / charsPerToken
			}
			for _, tc := range m.ToolCalls {
				for _, v := range tc.Input {
					total += len(fmt.Sprintf("%v", v)) / charsPerToken
				}
			}
		}
		return total
	}

	if estimate(messages) <= limit {
		return messages
	}

	// Drop oldest tool results first.
	const dropped = "[result dropped — conversation too long. You can re-read this file if needed.]"
	out := make([]ai.Message, len(messages))
	copy(out, messages)

	for i := range out {
		for j := range out[i].ToolResults {
			if out[i].ToolResults[j].Content != dropped {
				out[i].ToolResults[j].Content = dropped
				if estimate(out) <= limit {
					return out
				}
			}
		}
	}

	return out
}

// toolDetail returns the display string for a tool call (command or path).
func toolDetail(tc ai.ToolCall) string {
	switch tc.Name {
	case "bash":
		cmd, _ := tc.Input["command"].(string)
		return cmd
	case "read":
		path, _ := tc.Input["path"].(string)
		return path
	default:
		return ""
	}
}
