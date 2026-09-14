package agentui

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

type tuiState int

const (
	stateIdle                tuiState = iota
	stateThinking                     // waiting for model
	stateApproval                     // waiting for bash approval
	stateJudging                      // LLM judging a bash command
	stateAskPicker                    // waiting for ask answer
	stateHistSearch                   // Ctrl+R reverse search through prompt history
	stateCommandProposal              // model proposed a shell command; awaiting accept/dismiss/edit
	stateCommandProposalEdit          // user is editing the proposed shell command before accepting
	stateSlashPicker                  // slash-command sub-picker (e.g. /effort, /model)
	stateConfigEditor                 // /config two-level settings editor
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

	state        tuiState
	pending      pendingToolState
	histSearch   histSearchState
	slashPicker  *slashPicker
	configEditor *configEditor
}

type escTimeoutMsg struct{}

type initialPromptMsg struct {
	prompt string
}

// modelConfig is everything newModel needs to build a Model. It exists because
// the constructor had grown to nineteen positional parameters — long enough
// that three later additions were being assigned after construction instead,
// which split "what a Model starts as" across two places.
type modelConfig struct {
	// Session
	providerName string
	provider     provider.AgentClient
	modelID      string
	mode         sessionMode
	system       string
	toolDefs     []provider.ToolDef

	// Settings
	llmJudge    bool
	suggestions bool
	effort      string
	themeName   string
	closeKey    string
	hasDarkBg   bool

	// Starting content
	stdin          string
	initialDraft   string
	initialPrompt  string
	initialSession *history.Session

	// Persistence
	cfgPath       string
	sessionID     string
	promptHistory *history.PromptHistory
	sessionStore  *history.SessionStore
	exchanges     *history.ExchangeLog

	// Layout. -1 means the cursor row is unknown; clampAnchor pins to the
	// bottom on the first WindowSizeMsg.
	initialCursorRow int
}

// newModel constructs the TUI model.
func newModel(cfg modelConfig) Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.Placeholder = ""
	// Grow with the content (Shift/Alt+Enter and \-Enter insert newlines) up to
	// a cap, then scroll internally.
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = maxInputRows
	ta.SetValue(cfg.initialDraft)
	ta.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	contextWindow := 32_000
	if md := models.Find(cfg.providerName, cfg.modelID); md != nil && md.Context > 0 {
		contextWindow = md.Context
	}

	ctx, cancel := newGenerationContext()
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())

	sh := shell{
		provider:         cfg.provider,
		providerName:     cfg.providerName,
		modelID:          cfg.modelID,
		mode:             cfg.mode,
		system:           cfg.system,
		toolDefs:         cfg.toolDefs,
		contextWindow:    contextWindow,
		llmJudge:         cfg.llmJudge,
		suggestions:      cfg.suggestions,
		effort:           cfg.effort,
		initialPrompt:    cfg.initialPrompt,
		stdin:            cfg.stdin,
		input:            ta,
		spin:             sp,
		ctx:              ctx,
		cancel:           cancel,
		suggestionCtx:    suggestionCtx,
		suggestionCancel: suggestionCancel,
		renderer:         newRenderer(cfg.hasDarkBg, theme.Get(cfg.themeName)),
		hasDarkBg:        cfg.hasDarkBg,
		themeName:        cfg.themeName,
		closeKey:         cfg.closeKey,
		promptHistory:    cfg.promptHistory,
		histIdx:          -1,
		sessionStore:     cfg.sessionStore,
		sessionID:        cfg.sessionID,
		exchanges:        cfg.exchanges,
		cfgPath:          cfg.cfgPath,
		workDir:          workDirBasename(),
		cwd:              workDirFull(),
		tuiTopRow:        cfg.initialCursorRow,
	}
	// Cursor query failed; clampAnchor will pin to the bottom on first WindowSizeMsg.
	if cfg.initialCursorRow < 0 {
		sh.tuiTopRow = math.MaxInt32
	}
	sh.input.SetStyles(inputStyles(cfg.hasDarkBg, sh.renderer.styles.Suggestion))

	if cfg.initialSession != nil && len(cfg.initialSession.Messages) > 0 {
		sh.messages = cfg.initialSession.Messages
		sh.thread = deserializeThread(cfg.initialSession.Thread)
	} else if cfg.stdin != "" {
		sh.thread = append(sh.thread, UserEntry{
			Content: fmt.Sprintf("[stdin: %d lines]", strings.Count(cfg.stdin, "\n")+1),
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
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		if handled, newM, cmd := m.handleMouseMsg(msg); handled {
			return newM, cmd
		}
	}

	return m.updateViewportOnly(msg)
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
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}
	if m.closeKey != "" {
		if zsh, ok := keybinding.KeyMsgToZsh(msg.Key()); ok && zsh == m.closeKey {
			return m.quit()
		}
	}

	// When the help overlay is visible, any non-quit key closes it without further action.
	// This prevents keystrokes from silently reaching hidden inputs (e.g. the textinput).
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// ? opens the help overlay unless the user is typing a message in idle.
	if msg.String() == "?" {
		typingInIdle := m.state == stateIdle && m.input.Value() != ""
		if !typingInIdle {
			m.showHelp = true
			return m, nil
		}
	}

	if md := modeFor(m.state); md != nil {
		return md.handleKey(m, msg)
	}
	return m, nil
}

// modes maps each TUI state to the mode that owns key handling for it.
// Lookups return nil for unknown states (defensive — every defined state
// has an entry).
var modes = map[tuiState]mode{
	stateIdle:                idleMode{},
	stateThinking:            thinkingMode{},
	stateJudging:             thinkingMode{},
	stateApproval:            approvalMode{},
	stateCommandProposal:     commandProposalMode{},
	stateCommandProposalEdit: commandProposalEditMode{},
	stateAskPicker:           askPickerMode{},
	stateSlashPicker:         slashPickerMode{},
	stateConfigEditor:        configEditorMode{},
	stateHistSearch:          histSearchMode{},
}

func modeFor(s tuiState) mode { return modes[s] }

// applyModel swaps the active provider client to the given provider+model
// in memory only. Existing conversation messages are preserved. Use this for
// live preview while a picker is open; pair with persistActiveModel to write
// the selection to disk on commit.
func (m *Model) applyModel(providerName, modelID string) error {
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
	m.effort = pc.Effort
	m.contextWindow = 32_000
	if md := models.Find(providerName, modelID); md != nil && md.Context > 0 {
		m.contextWindow = md.Context
	}
	return nil
}

// persistActiveModel writes the currently active provider+model to the config
// file as the selected provider and that provider's default model.
func (m *Model) persistActiveModel() {
	if m.cfgPath == "" {
		return
	}
	cfg, exists, err := config.LoadConfig(m.cfgPath)
	if err != nil || !exists {
		return
	}
	pc := cfg.Providers[m.providerName]
	pc.Model = m.modelID
	cfg.Providers[m.providerName] = pc
	cfg.SelectedProvider = m.providerName
	_ = config.SaveConfig(m.cfgPath, cfg)
}

// applyTheme rebuilds the renderer with the given theme name.
func (m *Model) applyTheme(name string) {
	m.themeName = name
	m.renderer = newRenderer(m.hasDarkBg, theme.Get(name))
	m.input.SetStyles(inputStyles(m.hasDarkBg, m.renderer.styles.Suggestion))
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
	// Backslash-Enter is the portable newline fallback: a trailing "\" turns
	// Enter into a line break instead of a submit, for terminals that can't
	// report a real Shift+Enter.
	if v := m.input.Value(); strings.HasSuffix(v, "\\") {
		m.input.SetValue(v[:len(v)-1] + "\n")
		m.input.MoveToEnd()
		return m, nil
	}
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

func (m Model) denyPendingBash() (tea.Model, tea.Cmd) {
	m.appendThreadEntries(ToolResultEntry{Content: "User denied this command.", IsError: true})
	m.refreshViewport()
	return m.resumePendingToolLoop(m.pending.result("User denied this command.", true))
}

func (m Model) dismissCommandProposal() (tea.Model, tea.Cmd) {
	m.clearShellCommand()
	m.state = stateIdle
	m.refreshViewport()
	return m, m.startSuggestion()
}

// editCommandProposal loads the proposed command into the input for editing.
func (m Model) editCommandProposal() (tea.Model, tea.Cmd) {
	m.input.SetValue(m.shellCommand)
	m.input.CursorEnd()
	m.state = stateCommandProposalEdit
	return m, nil
}

// workDirFull returns the absolute working directory, or "." when it cannot be
// determined — ReadDir(".") then still lists something sensible.
func workDirFull() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
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
