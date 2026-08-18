package agentui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

type tuiState int

const (
	stateIdle            tuiState = iota
	stateThinking                 // waiting for model
	stateApproval                 // waiting for bash approval
	stateApprovalEdit             // user is editing a bash command before running
	stateJudging                  // LLM judging a bash command
	stateAskPicker                // waiting for ask answer
	stateHistSearch               // Ctrl+R reverse search through prompt history
	stateCommandProposal          // model proposed a shell command; awaiting accept/dismiss
	stateSlashPicker              // slash-command sub-picker (e.g. /effort, /theme, /model)
)

// histSearchState holds the state for Ctrl+R reverse search.
type histSearchState struct {
	query   string
	matches []string
	idx     int
}

// Model is the bubbletea model for the agent TUI. Shared, always-relevant
// state lives on the embedded shell; mode-local state (the active mode enum,
// pending tool, history-search buffer, open slash picker) lives here.
type Model struct {
	shell

	state       tuiState
	pending     pendingToolState
	histSearch  histSearchState
	slashPicker *slashPicker
}

type escTimeoutMsg struct{}

type initialPromptMsg struct {
	prompt string
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
	effort string,
	cfgPath string,
	initialDraft string,
	initialPrompt string,
	themeName string,
	closeKey string,
	promptHistory *history.PromptHistory,
	sessionStore *history.SessionStore,
	sessionID string,
	initialSession *history.Session,
	initialCursorRow int,
) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.Cursor.Style = r.NewStyle()
	ti.Cursor.TextStyle = r.NewStyle()
	ti.Cursor.SetMode(cursor.CursorStatic)
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

	sh := shell{
		provider:         prov,
		providerName:     providerName,
		modelID:          modelID,
		system:           system,
		contextWindow:    contextWindow,
		llmJudge:         llmJudge,
		suggestions:      suggestions,
		effort:           effort,
		initialPrompt:    initialPrompt,
		stdin:            stdin,
		input:            ti,
		spin:             sp,
		ctx:              ctx,
		cancel:           cancel,
		suggestionCtx:    suggestionCtx,
		suggestionCancel: suggestionCancel,
		renderer:         newRenderer(r, theme.Get(themeName), glamourStyle(r)),
		lipglossRenderer: r,
		themeName:        themeName,
		closeKey:         closeKey,
		promptHistory:    promptHistory,
		histIdx:          -1,
		sessionStore:     sessionStore,
		sessionID:        sessionID,
		cfgPath:          cfgPath,
		workDir:          workDirBasename(),
		tuiTopRow:        initialCursorRow,
	}
	// Cursor query failed; clampAnchor will pin to the bottom on first WindowSizeMsg.
	if initialCursorRow < 0 {
		sh.tuiTopRow = math.MaxInt32
	}
	sh.input.PlaceholderStyle = sh.renderer.styles.Suggestion

	if initialSession != nil && len(initialSession.Messages) > 0 {
		sh.messages = initialSession.Messages
		sh.thread = deserializeThread(initialSession.Thread)
	} else if stdin != "" {
		sh.thread = append(sh.thread, UserEntry{
			Content: fmt.Sprintf("[stdin: %d lines]", strings.Count(stdin, "\n")+1),
		})
	}

	return Model{shell: sh}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick}
	if prompt := strings.TrimSpace(m.initialPrompt); prompt != "" {
		cmds = append(cmds, func() tea.Msg { return initialPromptMsg{prompt: prompt} })
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.updateInner(msg)
	if mm, ok := newModel.(Model); ok {
		newModel = mm.clampAnchor()
	}
	return newModel, cmd
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case escTimeoutMsg:
		m.lastEscAt = time.Time{}
		return m, nil
	case copyToastTimeoutMsg:
		m.copyToastUntil = time.Time{}
		m.copyToastMsg = ""
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		if handled, newM, cmd := m.handleMouseMsg(msg); handled {
			return newM, cmd
		}
	}

	return m.updateViewportOnly(msg)
}

// handleMouseMsg handles URL clicks and drag-to-select. Wheel and other
// events fall through to the viewport.
// inputRowHeight returns the row count of the input section between the
// viewport's bottom separator and the status line's top separator.
func (m Model) inputRowHeight() int {
	switch m.state {
	case stateIdle:
		if matches := m.visibleSlashMatches(); len(matches) > 0 {
			n := min(len(matches), slashDropdownMaxRows)
			return 1 + n // dropdown rows + input line
		}
	case stateApproval, stateCommandProposal:
		return 4 // 3-line command block + 1 action-hints line
	case stateAskPicker:
		if m.pending.picker != nil {
			return strings.Count(m.pending.picker.View(m.renderer), "\n") + 1
		}
	case stateSlashPicker:
		if m.slashPicker != nil {
			return strings.Count(m.slashPicker.View(m.renderer), "\n") + 1
		}
	}
	return 1
}

// totalViewHeight returns the total row count of the View() output for the
// current state: header(1) + viewport + sep(1) + input + sep(1) + status(1).
func (m Model) totalViewHeight() int {
	_, vpH := m.viewportDims()
	return 4 + vpH + m.inputRowHeight()
}

// clampAnchor enforces tuiTopRow + totalViewHeight() <= m.height. When state
// inflates the rendered height past what fits below the current anchor, the
// terminal scrolls and tuiTopRow moves up to track that.
func (m Model) clampAnchor() Model {
	if m.height <= 0 {
		return m
	}
	totalH := m.totalViewHeight()
	maxTop := max(m.height-totalH, 0)
	if m.tuiTopRow > maxTop {
		m.tuiTopRow = maxTop
	}
	m.tuiTopRow = max(m.tuiTopRow, 0)
	return m
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

	// When the help overlay is visible, any non-quit key closes it without further action.
	// This prevents keystrokes from silently reaching hidden inputs (e.g. the textinput).
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// ? opens the help overlay when the user is not mid-message.
	if msg.String() == "?" {
		if m.state != stateIdle || m.input.Value() == "" {
			m.showHelp = true
			return m, nil
		}
	}

	if md := modeFor(m.state); md != nil {
		return md.handleKey(m, msg)
	}
	return m, nil
}

// modeFor returns the mode that owns key handling for the given state.
// Returns nil for unknown states (defensive — every defined state has a mode).
func modeFor(s tuiState) mode {
	switch s {
	case stateIdle:
		return idleMode{}
	case stateThinking, stateJudging:
		return thinkingMode{}
	case stateApproval:
		return approvalMode{}
	case stateApprovalEdit:
		return approvalEditMode{}
	case stateCommandProposal:
		return commandProposalMode{}
	case stateAskPicker:
		return askPickerMode{}
	case stateSlashPicker:
		return slashPickerMode{}
	case stateHistSearch:
		return histSearchMode{}
	}
	return nil
}

// switchModel swaps the active provider client to the given provider+model.
// It loads the on-disk config to find auth for the target provider, rebuilds
// the AgentClient, updates session fields, and persists the new selection.
// Existing conversation messages are preserved.
func (m *Model) switchModel(providerName, modelID string) error {
	if m.cfgPath == "" {
		return fmt.Errorf("no config path available to resolve auth for %q", providerName)
	}
	cfg, exists, err := config.LoadConfig(m.cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !exists {
		return fmt.Errorf("config file not found")
	}
	pc, ok := cfg.Providers[providerName]
	if !ok {
		return fmt.Errorf("provider %q has no config block — run `tw config` to add it", providerName)
	}
	auth, err := config.ResolveAuth(providerName, pc)
	if err != nil {
		return fmt.Errorf("resolving auth for %q: %w", providerName, err)
	}
	client, err := config.NewClientFromResolved(config.ResolvedConfig{
		ProviderName: providerName,
		Model:        modelID,
		APIKey:       auth.APIKey,
		BaseURL:      auth.BaseURL,
	})
	if err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	m.provider = client
	m.providerName = providerName
	m.modelID = modelID
	m.contextWindow = 32_000
	if md := models.Find(providerName, modelID); md != nil && md.Context > 0 {
		m.contextWindow = md.Context
	}

	cfg.SelectedProvider = providerName
	pc.Model = modelID
	cfg.Providers[providerName] = pc
	_ = config.SaveConfig(m.cfgPath, cfg)
	return nil
}

// applyTheme rebuilds the renderer with the given theme name.
func (m *Model) applyTheme(name string) {
	m.themeName = name
	if m.lipglossRenderer != nil {
		m.renderer = newRenderer(m.lipglossRenderer, theme.Get(name), glamourStyle(m.lipglossRenderer))
		m.input.PlaceholderStyle = m.renderer.styles.Suggestion
	}
}

// openSlashPicker activates the slash-picker sub-TUI for the given command.
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
	if strings.HasPrefix(text, "/") {
		return m.dispatchSlashCommand(text)
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

func (m Model) denyPendingBash() (tea.Model, tea.Cmd) {
	m.appendThreadEntries(ToolResultEntry{Content: "User denied this command.", IsError: true})
	m.refreshViewport()
	return m.resumePendingToolLoop(m.pending.result("User denied this command.", true))
}

func (m Model) acceptCommandProposal() (tea.Model, tea.Cmd) {
	return m.quit()
}

func (m Model) dismissCommandProposal() (tea.Model, tea.Cmd) {
	m.clearShellCommand()
	m.state = stateIdle
	m.refreshViewport()
	return m, m.startSuggestion()
}

// needsApproval returns true if the bash command must be confirmed by the user.
func (m Model) needsApproval(cmd string) bool {
	return allowlist.NeedsApproval(nil, cmd)
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
		Effort:   m.effectiveEffort(),
	}
}

// effectiveEffort returns the effort to send: configured value, or the default
// for thinking-capable models, or empty for models without thinking support.
func (m Model) effectiveEffort() string {
	return config.EffectiveEffort(m.providerName, m.modelID, m.effort)
}

// refreshViewport re-renders the thread, updates viewport content, and
// re-indexes URL positions for click-to-open handling.
func (m *Model) refreshViewport() {
	content := m.renderer.RenderThread(m.thread)
	if content == "" {
		content = m.emptyStateHint()
	}
	m.vp.SetContent(content)
	m.links = findLinks(content)
	if !m.userScrolled {
		m.vp.GotoBottom()
	}
}

// emptyStateHint returns the placeholder text shown when the thread is empty.
func (m Model) emptyStateHint() string {
	dim := m.renderer.styles.ActionHints
	key := m.renderer.styles.HelpKey
	return dim.Render("  Ready. Ask anything, or type ") +
		key.Render("/") +
		dim.Render(" for commands.")
}

// popupHeight is the maximum number of terminal lines the TUI occupies.
// Inline mode (no alt screen) renders in place, so we cap the height to
// keep it feeling like a sized popup rather than a full-page takeover.
const popupHeight = 30

// viewportDims calculates viewport dimensions from terminal size.
func (m Model) viewportDims() (width, height int) {
	// Layout: header(1) | viewport | sep(1) | input(1) | sep(1) | status(1)
	innerW := max(m.width, 10)
	h := popupHeight
	if m.height > 0 && m.height < h {
		h = m.height
	}
	innerH := max(h-5, 3) // header(1) + sep(1) + input(1) + sep(1) + status(1)
	return innerW, innerH
}

func workDirBasename() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Base(wd)
}

func (m Model) modelShortName() string {
	md := models.Find(m.providerName, m.modelID)
	if md == nil {
		return m.modelID
	}
	return strings.TrimPrefix(md.Name, "Claude ")
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
	if pe, ok := errors.AsType[*provider.Error](err); ok {
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
