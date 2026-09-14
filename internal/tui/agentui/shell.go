package agentui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"

	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// shell holds state shared across every TUI mode. Mode-specific state (the
// active mode enum, pending tool, history search buffer, open slash picker)
// stays on Model.
type shell struct {
	// Session config
	provider      provider.AgentClient
	providerName  string
	modelID       string
	mode          sessionMode
	system        string
	toolDefs      []provider.ToolDef
	contextWindow int

	// Settings
	llmJudge      bool
	suggestions   bool
	effort        string
	initialPrompt string

	// Conversation
	messages     []provider.Message
	thread       []ThreadEntry
	stdin        string
	inputTokens  int
	outputTokens int

	// Always-on components
	vp    viewport.Model
	input textarea.Model
	spin  spinner.Model

	// Layout
	width        int
	height       int
	ready        bool
	userScrolled bool
	showHelp     bool
	tuiTopRow    int

	// In-flight generation
	turnStartedAt    time.Time // when the current turn began, for the elapsed counter
	activity         string    // what the agent is doing right now, shown while thinking
	ctx              context.Context
	cancel           context.CancelFunc
	nextGeneration   uint64
	activeGeneration uint64

	// Sidecar suggestion engine
	suggestion       string
	suggestionCtx    context.Context
	suggestionCancel context.CancelFunc
	nextSuggestion   uint64
	activeSuggestion uint64

	// Rendering
	renderer  Renderer
	hasDarkBg bool // detected terminal background; used to rebuild styles on theme change
	themeName string

	// Lifecycle / exit
	closeKey     string
	quitting     bool
	shellCommand string
	lastEscAt    time.Time

	// History navigation (always available from idle)
	promptHistory *history.PromptHistory
	histIdx       int
	histDraft     string

	// Session persistence
	sessionStore *history.SessionStore
	sessionID    string

	// Append-only record of completed turns, for judging output quality later.
	exchanges *history.ExchangeLog

	// Paths
	cfgPath string
	workDir string
	// cwd is the full working directory, resolved once at construction. The
	// @-dropdown lists relative to it on every keystroke, so it must not be an
	// os.Getwd() call per key.
	cwd string

	// Always-on completion dropdowns
	slashSel completionState
	atSel    completionState

	// Mouse / selection / copy toast (always-on widgets)
	links          []linkPosition
	selection      selectionState
	copyToastUntil time.Time
	copyToastMsg   string
}
