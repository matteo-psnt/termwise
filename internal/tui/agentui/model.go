package agentui

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/keybinding"
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

// isBusy reports whether a turn is in flight: waiting on the model or on the
// LLM safety judge. These are the states the spinner animates in.
func (m Model) isBusy() bool {
	return m.state == stateThinking || m.state == stateJudging
}

// histSearchState holds the state for Ctrl+R reverse search.
type histSearchState struct {
	query   string
	matches []string
	idx     int
}

// Model is the bubbletea model for the agent TUI. Shared, always-relevant
// state lives on the embedded sharedState; mode-local state (the active mode enum,
// pending tool, history-search buffer, open slash picker) lives here.
type Model struct {
	sharedState

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

	ctx, cancel := context.WithCancel(context.Background())
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())

	sh := sharedState{
		provider:         cfg.provider,
		providerName:     cfg.providerName,
		modelID:          cfg.modelID,
		sessionMode:      cfg.mode,
		system:           cfg.system,
		toolDefs:         cfg.toolDefs,
		contextWindow:    contextWindowFor(cfg.providerName, cfg.modelID),
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
		renderer:         newRenderer(cfg.hasDarkBg, theme.Get(cfg.themeName), 0),
		hasDarkBg:        cfg.hasDarkBg,
		themeName:        cfg.themeName,
		closeKey:         cfg.closeKey,
		promptHistory:    cfg.promptHistory,
		histIdx:          -1,
		sessionStore:     cfg.sessionStore,
		sessionID:        cfg.sessionID,
		exchanges:        cfg.exchanges,
		cfgPath:          cfg.cfgPath,
		cwd:              workDirFull(),
		slashCache:       &slashMatchCache{},
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

	return Model{sharedState: sh}
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
	next, cmd := m.updateInner(msg)
	if mm, ok := next.(Model); ok {
		next = mm.clampAnchor()
	}
	return next, cmd
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

	return m.forwardToComponents(msg)
}

// forwardToComponents passes on a message no handler claimed (cursor blink,
// wheel scroll and the like): always to the input, and to the viewport in the
// states that let the thread scroll.
func (m Model) forwardToComponents(msg tea.Msg) (tea.Model, tea.Cmd) {
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	if m.state != stateIdle && !m.isBusy() && m.state != stateCommandProposal {
		return m, inputCmd
	}
	var vpCmd tea.Cmd
	m.vp, vpCmd = m.vp.Update(msg)
	// Track wheel scrolling like PgUp/PgDn: without it, the next refresh
	// snapped a reader scrolled up by wheel back to the bottom.
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		m.userScrolled = !m.vp.AtBottom()
	}
	return m, tea.Batch(vpCmd, inputCmd)
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

// abort quits without accepting a pending command proposal. The proposal
// screen has its own accept key; Ctrl+C and the close key mean "get me out",
// and handing the unaccepted command to the shell widget would overwrite the
// line the user had typed with a command they never chose (or, from edit
// mode, with the original rather than their edit).
func (m Model) abort() (tea.Model, tea.Cmd) {
	m.clearShellCommand()
	return m.quit()
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

// exitShellCommand is the command the model proposed. Whatever it holds when
// the TUI exits is handed to the shell widget as the result.
func (m Model) exitShellCommand() string {
	return m.shellCommand
}

// handleKey handles all keyboard input based on current state.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.abort()
	}
	if m.closeKey != "" {
		if zsh, ok := keybinding.KeyMsgToZsh(msg.Key()); ok && zsh == m.closeKey {
			return m.abort()
		}
	}

	// When the help overlay is visible, any non-quit key closes it without further action.
	// This prevents keystrokes from silently reaching hidden inputs (e.g. the textinput).
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// ? opens the help overlay unless the user is typing text it belongs in.
	if msg.String() == "?" && !m.typingText() {
		m.showHelp = true
		return m, nil
	}

	if md := modeFor(m.state); md != nil {
		return md.handleKey(m, msg)
	}
	return m, nil
}

// typingText reports whether a printable key is text the user is
// entering rather than a command: a message in idle, the command being edited
// (globs like "file?.txt"), a Ctrl+R query, the ask picker's free-text "Other"
// answer, or a keybinding being captured. Help must not swallow "?" there.
func (m Model) typingText() bool {
	switch m.state {
	case stateIdle:
		return m.input.Value() != ""
	case stateCommandProposalEdit, stateHistSearch:
		return true
	case stateAskPicker:
		return m.pending.picker != nil && m.pending.picker.onOther()
	case stateConfigEditor:
		return m.configEditor != nil && m.configEditor.phase == configPhaseCapture
	}
	return false
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

// workDirFull returns the absolute working directory, or "." when it cannot be
// determined — ReadDir(".") then still lists something sensible.
func workDirFull() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
