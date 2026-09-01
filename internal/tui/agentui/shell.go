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
	system        string
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

	// Paths
	cfgPath string
	workDir string

	// Always-on slash dropdown
	slashCursor int
	slashClosed bool

	// Mouse / selection / copy toast (always-on widgets)
	links          []linkPosition
	selection      selectionState
	copyToastUntil time.Time
	copyToastMsg   string
}
