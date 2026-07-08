package agentui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

type tuiState int

const (
	stateIdle       tuiState = iota
	stateThinking            // waiting for model
	stateApproval            // waiting for bash approval
	stateJudging             // LLM judging a bash command
	stateAskPicker           // waiting for ask answer
	stateHistSearch          // Ctrl+R reverse search through prompt history
)

// histSearchState holds the state for Ctrl+R reverse search.
type histSearchState struct {
	query   string
	matches []string
	idx     int
}

// Model is the bubbletea model for the agent TUI.
type Model struct {
	// Session config
	provider      provider.AgentClient
	modelID       string
	system        string
	contextWindow int

	// Config
	llmJudge    bool
	suggestions bool
	// initialPrompt is auto-submitted when the TUI starts.
	initialPrompt string

	// Conversation
	messages []provider.Message
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
	width        int
	height       int
	ready        bool
	userScrolled bool // true when user has manually scrolled up

	// Cancellation for the current in-flight async generation.
	ctx              context.Context
	cancel           context.CancelFunc
	nextGeneration   uint64
	activeGeneration uint64

	// Sidecar prompt suggestion generation.
	suggestion       string
	suggestionCtx    context.Context
	suggestionCancel context.CancelFunc
	nextSuggestion   uint64
	activeSuggestion uint64

	// Renderer (built once)
	renderer Renderer

	// closeKey is the zsh bindkey string (e.g. "^T") that quits the TUI,
	// matching the shell keybinding that opened it.
	closeKey string

	// quitting is set before tea.Quit so View() returns "" on the final frame,
	// causing bubbletea's inline renderer to clear all drawn lines on exit.
	quitting bool

	// Prompt history navigation.
	promptHistory *history.PromptHistory
	histIdx       int    // index into history; -1 means not navigating
	histDraft     string // saved draft before navigating

	// Ctrl+R reverse search.
	histSearch histSearchState

	// Session persistence.
	sessionStore *history.SessionStore
	sessionID    string
	providerName string

	// shellCommand holds the latest completed command that can be returned to a
	// shell IPC caller when the TUI exits.
	shellCommand string
}

type generationMsg struct {
	generation uint64
	msg        tea.Msg
}

type initialPromptMsg struct {
	prompt string
}

func newGenerationContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func newSuggestionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

func wrapGenerationCmd(generation uint64, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if msg == nil {
			return nil
		}
		return generationMsg{generation: generation, msg: msg}
	}
}

// newModel constructs the TUI model.
func newModel(
	providerName string,
	prov provider.AgentClient,
	modelID string,
	system string,
	stdin string,
	r *lipgloss.Renderer,
	llmJudge bool,
	suggestions bool,
	initialDraft string,
	initialPrompt string,
	themeName string,
	closeKey string,
	promptHistory *history.PromptHistory,
	sessionStore *history.SessionStore,
	sessionID string,
	initialSession *history.Session,
) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.SetValue(initialDraft)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	contextWindow := 32_000
	if md := models.Find(providerName, modelID); md != nil && md.Context > 0 {
		contextWindow = md.Context
	}

	ctx, cancel := newGenerationContext()
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())

	m := Model{
		provider:         prov,
		modelID:          modelID,
		system:           system,
		stdin:            stdin,
		contextWindow:    contextWindow,
		llmJudge:         llmJudge,
		suggestions:      suggestions,
		initialPrompt:    initialPrompt,
		input:            ti,
		spin:             sp,
		ctx:              ctx,
		cancel:           cancel,
		suggestionCtx:    suggestionCtx,
		suggestionCancel: suggestionCancel,
		renderer:         newRenderer(r, theme.Get(themeName), glamourStyle(r)),
		closeKey:         closeKey,
		promptHistory:    promptHistory,
		histIdx:          -1,
		sessionStore:     sessionStore,
		sessionID:        sessionID,
		providerName:     providerName,
	}
	m.input.PlaceholderStyle = m.renderer.styles.Suggestion

	if initialSession != nil && len(initialSession.Messages) > 0 {
		m.messages = initialSession.Messages
		m.thread = deserializeThread(initialSession.Thread)
	} else if stdin != "" {
		m.thread = append(m.thread, UserEntry{
			Content: fmt.Sprintf("[stdin: %d lines]", strings.Count(stdin, "\n")+1),
		})
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, textinput.Blink}
	if prompt := strings.TrimSpace(m.initialPrompt); prompt != "" {
		cmds = append(cmds, func() tea.Msg { return initialPromptMsg{prompt: prompt} })
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)
	case generationMsg:
		return m.handleGenerationMsg(msg)
	case suggestionMsg:
		return m.handleSuggestionMsg(msg)
	case initialPromptMsg:
		return m.handleInitialPrompt(msg.prompt)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m.updateViewportOnly(msg)
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.suggestionCancel()
	m.cancel()
	m.saveSession()
	m.quitting = true
	return m, tea.Quit
}

// saveSession persists the current conversation to disk if a session ID is set.
func (m *Model) saveSession() {
	if m.sessionStore == nil || m.sessionID == "" || len(m.messages) == 0 {
		return
	}
	s := history.Session{
		ID:           m.sessionID,
		ProviderName: m.providerName,
		ModelID:      m.modelID,
		Messages:     m.messages,
		Thread:       serializeThread(m.thread),
	}
	m.sessionStore.Save(s) //nolint:errcheck // Session persistence is best-effort during UI updates.
}

func (m *Model) setShellCommand(content string) {
	m.shellCommand = strings.TrimSpace(content)
}

func (m *Model) clearShellCommand() {
	m.shellCommand = ""
}

func (m Model) exitShellCommand() string {
	return m.shellCommand
}

// handleKey handles all keyboard input based on current state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}
	if m.closeKey != "" {
		if zsh, ok := keybinding.KeyMsgToZsh(msg); ok && zsh == m.closeKey {
			return m.quit()
		}
	}

	switch m.state {
	case stateIdle:
		return m.handleIdleKey(msg)
	case stateThinking, stateJudging:
		return m.handleThinkingKey(msg)
	case stateApproval:
		return m.handleApprovalKey(msg)
	case stateAskPicker:
		return m.handlePickerKey(msg)
	case stateHistSearch:
		return m.handleHistSearchKey(msg)
	}
	return m, nil
}

func (m Model) handleThinkingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyPgUp:
		m.vp.PageUp()
		if m.vp.ScrollPercent() < 1.0 {
			m.userScrolled = true
		}
		return m, nil
	case tea.KeyPgDown:
		m.vp.PageDown()
		if m.vp.ScrollPercent() >= 1.0 {
			m.userScrolled = false
		}
		return m, nil
	case tea.KeyEnd:
		m.userScrolled = false
		m.vp.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		m.interruptActiveTurn()
		return m, nil
	}
	return m, nil
}

func (m Model) handleIdleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.submitCurrentInput()

	case tea.KeyTab:
		if s := m.visibleSuggestion(); s != "" {
			m.input.SetValue(s)
			m.input.CursorEnd()
			m.suggestion = ""
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case tea.KeyUp:
		return m.historyBack(), nil

	case tea.KeyDown:
		return m.historyForward(), nil

	case tea.KeyPgUp:
		m.vp.PageUp()
		if m.vp.ScrollPercent() < 1.0 {
			m.userScrolled = true
		}
		return m, nil

	case tea.KeyPgDown:
		m.vp.PageDown()
		if m.vp.ScrollPercent() >= 1.0 {
			m.userScrolled = false
		}
		return m, nil

	case tea.KeyEnd:
		m.userScrolled = false
		m.vp.GotoBottom()
		return m, nil

	case tea.KeyEsc:
		// ESC while idle does nothing.
		return m, nil

	default:
		if msg.String() == "ctrl+r" {
			return m.enterHistSearch(), nil
		}
		// Any other typing cancels history navigation.
		if m.histIdx >= 0 {
			m.histIdx = -1
			m.histDraft = ""
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) handleInitialPrompt(prompt string) (tea.Model, tea.Cmd) {
	if m.state != stateIdle {
		return m, nil
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return m, nil
	}
	m.initialPrompt = ""
	if m.promptHistory != nil {
		m.promptHistory.Push(prompt) //nolint:errcheck // Prompt history should not block sending a message.
	}
	return m.submitMessage(prompt)
}

func (m Model) submitCurrentInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.SetValue("")
	m.histIdx = -1
	m.histDraft = ""
	if m.promptHistory != nil {
		m.promptHistory.Push(text) //nolint:errcheck // Prompt history should not block sending a message.
	}
	return m.submitMessage(text)
}

// historyBack moves one step older in prompt history.
func (m Model) historyBack() Model {
	if m.promptHistory == nil {
		return m
	}
	entries := m.promptHistory.Entries()
	if len(entries) == 0 {
		return m
	}
	if m.histIdx < 0 {
		// Save the current draft before navigating.
		m.histDraft = m.input.Value()
	}
	next := m.histIdx + 1
	if next >= len(entries) {
		return m
	}
	m.histIdx = next
	m.input.SetValue(entries[m.histIdx])
	m.input.CursorEnd()
	return m
}

// historyForward moves one step newer in prompt history, restoring the draft
// when moving past the most recent entry.
func (m Model) historyForward() Model {
	if m.histIdx < 0 {
		return m
	}
	m.histIdx--
	if m.histIdx < 0 {
		m.input.SetValue(m.histDraft)
		m.histDraft = ""
	} else if m.promptHistory != nil {
		entries := m.promptHistory.Entries()
		if m.histIdx < len(entries) {
			m.input.SetValue(entries[m.histIdx])
		}
	}
	m.input.CursorEnd()
	return m
}

// enterHistSearch switches to stateHistSearch and seeds the initial match list.
func (m Model) enterHistSearch() Model {
	m.histDraft = m.input.Value()
	m.histSearch = histSearchState{}
	m.state = stateHistSearch
	// Seed matches from current input value as query.
	m.histSearch.query = m.histDraft
	m.histSearch.matches = m.filterHistory(m.histSearch.query)
	m.histSearch.idx = 0
	if len(m.histSearch.matches) > 0 {
		m.input.SetValue(m.histSearch.matches[0])
		m.input.CursorEnd()
	}
	return m
}

// handleHistSearchKey handles keyboard input while in Ctrl+R search mode.
func (m Model) handleHistSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Accept the current match and return to idle.
		m.state = stateIdle
		m.histSearch = histSearchState{}
		m.input.CursorEnd()
		return m, nil

	case tea.KeyEsc:
		// Cancel search: restore original draft.
		m.state = stateIdle
		m.input.SetValue(m.histDraft)
		m.histSearch = histSearchState{}
		m.histDraft = ""
		m.input.CursorEnd()
		return m, nil

	case tea.KeyUp, tea.KeyDown:
		// Cycle through matches.
		if len(m.histSearch.matches) == 0 {
			return m, nil
		}
		if msg.Type == tea.KeyUp {
			m.histSearch.idx++
			if m.histSearch.idx >= len(m.histSearch.matches) {
				m.histSearch.idx = len(m.histSearch.matches) - 1
			}
		} else {
			m.histSearch.idx--
			if m.histSearch.idx < 0 {
				m.histSearch.idx = 0
			}
		}
		m.input.SetValue(m.histSearch.matches[m.histSearch.idx])
		m.input.CursorEnd()
		return m, nil

	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.histSearch.query) > 0 {
			m.histSearch.query = m.histSearch.query[:len(m.histSearch.query)-1]
		}
		m.histSearch.matches = m.filterHistory(m.histSearch.query)
		m.histSearch.idx = 0
		if len(m.histSearch.matches) > 0 {
			m.input.SetValue(m.histSearch.matches[0])
		} else {
			m.input.SetValue(m.histSearch.query)
		}
		m.input.CursorEnd()
		return m, nil

	default:
		if msg.String() == "ctrl+r" {
			// Ctrl+R again: cycle to next match.
			if len(m.histSearch.matches) > 0 {
				m.histSearch.idx = (m.histSearch.idx + 1) % len(m.histSearch.matches)
				m.input.SetValue(m.histSearch.matches[m.histSearch.idx])
				m.input.CursorEnd()
			}
			return m, nil
		}
		// Printable character: extend the search query.
		if len(msg.Runes) > 0 {
			m.histSearch.query += string(msg.Runes)
			m.histSearch.matches = m.filterHistory(m.histSearch.query)
			m.histSearch.idx = 0
			if len(m.histSearch.matches) > 0 {
				m.input.SetValue(m.histSearch.matches[0])
			} else {
				m.input.SetValue(m.histSearch.query)
			}
			m.input.CursorEnd()
		}
		return m, nil
	}
}

// filterHistory returns history entries that contain query as a substring,
// in newest-first order.
func (m Model) filterHistory(query string) []string {
	if m.promptHistory == nil {
		return nil
	}
	entries := m.promptHistory.Entries()
	if query == "" {
		return entries
	}
	var out []string
	q := strings.ToLower(query)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e), q) {
			out = append(out, e)
		}
	}
	return out
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
	m.resetSuggestionContext()
	m.clearSuggestion()
	m.clearShellCommand()

	// Attach stdin to first user message if present.
	content := text
	if m.stdin != "" {
		content = text + "\n\n" + m.stdin
		m.stdin = ""
	}

	m.thread = append(m.thread, UserEntry{Content: text})
	m.messages = append(m.messages, provider.Message{Role: "user", Content: content})
	m.messages = trimContext(m.messages, m.contextWindow)
	m.beginGeneration()
	m.state = stateThinking
	m.refreshViewport()

	return m, m.startChat()
}

// chatRequest builds the ChatRequest from current state.
func (m Model) chatRequest() provider.ChatRequest {
	return provider.ChatRequest{
		Model:    m.modelID,
		System:   m.system,
		Messages: m.messages,
		Tools:    tools.Defs,
	}
}

func (m *Model) beginGeneration() {
	m.resetGenerationContext()
	m.nextGeneration++
	m.activeGeneration = m.nextGeneration
}

func (m *Model) resetGenerationContext() {
	m.cancel()
	m.ctx, m.cancel = newGenerationContext()
}

func (m *Model) startChat() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		m.wrapActiveGeneration(agent.ChatCmd(m.ctx, m.provider, m.chatRequest())),
	)
}

func (m *Model) interruptActiveTurn() {
	m.resetGenerationContext()
	m.finishGeneration()
	m.clearPendingTool()
	m.state = stateIdle
	m.appendThreadEntries(ErrorEntry{Content: "Request interrupted."})
	m.refreshViewport()
}

func (m *Model) finishGeneration() {
	m.activeGeneration = 0
}

func (m Model) wrapActiveGeneration(cmd tea.Cmd) tea.Cmd {
	return wrapGenerationCmd(m.activeGeneration, cmd)
}

func (m Model) handleGenerationMsg(msg generationMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.activeGeneration {
		return m, nil
	}

	switch inner := msg.msg.(type) {
	case agent.ResponseMsg:
		return m.handleResponseMsg(inner)
	case agent.ToolExecutedMsg:
		return m.handleToolExecutedMsg(inner)
	case agent.NeedsApprovalMsg:
		return m.handleNeedsApprovalMsg(inner)
	case agent.AskMsg:
		return m.handleAskMsg(inner)
	case agent.RespondMsg:
		return m.handleRespondMsg(inner)
	case agent.AllToolsDoneMsg:
		return m.handleAllToolsDoneMsg(inner)
	case judgmentMsg:
		return m.handleJudgmentMsg(inner)
	default:
		return m, nil
	}
}

// refreshViewport re-renders the thread and updates viewport content.
func (m *Model) refreshViewport() {
	content := m.renderer.RenderThread(m.thread)
	m.vp.SetContent(content)
	if !m.userScrolled {
		m.vp.GotoBottom()
	}
}

// handleMouse handles mouse events: wheel scrolling and title-bar clicks to go to bottom.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Left-click on the title bar releases the scroll lock and snaps to bottom.
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == 0 && m.userScrolled {
		m.userScrolled = false
		m.vp.GotoBottom()
		return m, nil
	}

	var vpCmd tea.Cmd
	m.vp, vpCmd = m.vp.Update(msg)

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.vp.ScrollPercent() < 1.0 {
			m.userScrolled = true
		}
	case tea.MouseButtonWheelDown:
		if m.vp.ScrollPercent() >= 1.0 {
			m.userScrolled = false
		}
	}

	return m, vpCmd
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
func trimContext(messages []provider.Message, contextWindow int) []provider.Message {
	const (
		fixedTokens   = 700 // system prompt + tool defs estimate
		charsPerToken = 4
		threshold     = 0.70
	)

	limit := int(float64(contextWindow) * threshold)

	estimate := func(msgs []provider.Message) int {
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
	out := make([]provider.Message, len(messages))
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
	var pe *provider.Error
	if errors.As(err, &pe) {
		switch pe.Kind {
		case provider.ErrAuth:
			return pe.Reason + " Run `tw config` to update your key."
		case provider.ErrModelNotFound:
			return pe.Reason + " Run `tw config` to change the model."
		}
	}
	return err.Error()
}

// toolDetail returns the display string for a tool call (command or path).
func toolDetail(tc provider.ToolCall) string {
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
