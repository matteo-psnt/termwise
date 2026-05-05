package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/configtui"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/matteo-psnt/termwise/internal/tools"
)

type tuiState int

const (
	stateIdle      tuiState = iota
	stateThinking           // waiting for model
	stateApproval           // waiting for bash approval
	stateJudging            // LLM judging a bash command
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

	// Config
	llmJudge bool

	// Conversation
	messages []ai.Message
	thread   []ThreadEntry
	stdin    string // pre-loaded stdin, attached to first message

	// State
	state tuiState

	// Pending tool loop (used during stateApproval / stateAskPicker)
	pending pendingToolState

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

	// Renderer (built once)
	renderer Renderer

	// closeKey is the zsh bindkey string (e.g. "^T") that quits the TUI,
	// matching the shell keybinding that opened it.
	closeKey string

	// quitting is set before tea.Quit so View() returns "" on the final frame,
	// causing bubbletea's inline renderer to clear all drawn lines on exit.
	quitting bool
}

// newModel constructs the TUI model.
func newModel(
	providerName string,
	provider ai.AgentProvider,
	modelID string,
	system string,
	stdin string,
	r *lipgloss.Renderer,
	llmJudge bool,
	prefill string,
	themeName string,
	closeKey string,
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
		llmJudge:      llmJudge,
		input:         ti,
		spin:          sp,
		ctx:           ctx,
		cancel:        cancel,
		renderer: Renderer{
			styles:  newStyles(r, theme.Get(themeName)),
			glamour: glamourStyle(r),
		},
		closeKey:      closeKey,
	}

	if stdin != "" {
		m.thread = append(m.thread, UserEntry{
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
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)
	case agent.ResponseMsg:
		return m.handleResponseMsg(msg)
	case agent.ToolExecutedMsg:
		return m.handleToolExecutedMsg(msg)
	case agent.NeedsApprovalMsg:
		return m.handleNeedsApprovalMsg(msg)
	case agent.AskMsg:
		return m.handleAskMsg(msg)
	case agent.RespondMsg:
		return m.handleRespondMsg(msg)
	case agent.AllToolsDoneMsg:
		return m.handleAllToolsDoneMsg(msg)
	case judgmentMsg:
		return m.handleJudgmentMsg(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m.updateViewportOnly(msg)
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Quit
}

// handleKey handles all keyboard input based on current state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}
	if m.closeKey != "" {
		if zsh, ok := configtui.KeyMsgToZsh(msg); ok && zsh == m.closeKey {
			return m.quit()
		}
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
		return m.approvePendingBash()

	case tea.KeyEsc:
		m.appendThreadEntries(ToolResultEntry{Content: "User denied this command.", IsError: true})
		m.refreshViewport()
		return m.resumePendingToolLoop(m.pending.result("User denied this command.", true))
	}
	return m, nil
}

// needsApproval returns true if the bash command must be confirmed by the user.
func (m Model) needsApproval(cmd string) bool {
	return allowlist.NeedsApproval(nil, cmd)
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending.picker == nil {
		return m, nil
	}
	updated, result, submitted, cancelled := m.pending.picker.Update(msg)
	m.pending.picker = &updated

	if cancelled {
		result = "User cancelled"
		submitted = true
	}

	if submitted {
		m.appendThreadEntries(UserEntry{Content: result})
		m.refreshViewport()
		return m.resumePendingToolLoop(m.pending.result(result, false))
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

	m.thread = append(m.thread, UserEntry{Content: text})
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
	content := m.renderer.RenderThread(m.thread)
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

// providerErrMsg returns the error string with contextual advice appended for
// known recoverable error kinds (auth failure, model not found).
func providerErrMsg(err error) string {
	var pe *ai.ProviderError
	if errors.As(err, &pe) {
		switch pe.Kind {
		case ai.ErrAuth:
			return pe.Reason + " Run `tw config` to update your key."
		case ai.ErrModelNotFound:
			return pe.Reason + " Run `tw config` to change the model."
		}
	}
	return err.Error()
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
