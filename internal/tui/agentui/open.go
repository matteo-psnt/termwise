package agentui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/envcontext"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// sessionMode selects which agent the TUI runs. The default agent generates
// commands; explain mode breaks down a command the user already has and is
// given no command tool at all, so it cannot drift back into proposing one.
type sessionMode int

const (
	modeAgent sessionMode = iota
	modeExplain
)

// systemAndTools returns the system prompt and tool set for the mode.
func (mode sessionMode) systemAndTools(env envcontext.Context) (string, []provider.ToolDef) {
	if mode == modeExplain {
		return systemprompt.Explain(agenttools.ExplainDefs, env), agenttools.ExplainDefs
	}
	return systemprompt.Agent(agenttools.Defs, env), agenttools.Defs
}

type openProgramConfig struct {
	mode          sessionMode
	providerName  string
	provider      provider.AgentClient
	modelID       string
	cfgPath       string
	stdin         string
	stdinPiped    bool
	initialDraft  string
	initialPrompt string
	sessionID     string
	forceResume   bool
	programInput  *os.File
	programOutput *os.File
}

// Open launches the agent TUI without an initial prompt.
func Open(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	sessionID string,
	forceResume bool,
) error {
	return OpenWithPrompt(providerName, provider, modelID, cfgPath, "", sessionID, forceResume)
}

// OpenWithPrompt launches the agent TUI and submits an initial prompt. An empty
// prompt just opens the TUI.
func OpenWithPrompt(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	initialPrompt string,
	sessionID string,
	forceResume bool,
) error {
	stdin, stdinPiped, err := readProgramStdin()
	if err != nil {
		return err
	}
	_, err = openProgram(openProgramConfig{
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		cfgPath:       cfgPath,
		stdin:         stdin,
		stdinPiped:    stdinPiped,
		initialPrompt: initialPrompt,
		sessionID:     sessionID,
		forceResume:   forceResume,
	})
	return err
}

// OpenExplain launches the agent TUI in explain mode. An empty initialPrompt
// opens the input box with an explain-mode hint instead of submitting, which is
// what bare `tw explain` does.
func OpenExplain(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	initialPrompt string,
	sessionID string,
	forceResume bool,
) error {
	stdin, stdinPiped, err := readProgramStdin()
	if err != nil {
		return err
	}
	// `fc -ln -1 | tw explain` has already said what to explain. Asking the user
	// to type something first would make the pipe pointless; submitMessage
	// appends the stdin to this, giving the same shape as the argument form.
	if initialPrompt == "" && stdin != "" {
		initialPrompt = "Explain this command:"
	}
	_, err = openProgram(openProgramConfig{
		mode:          modeExplain,
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		cfgPath:       cfgPath,
		stdin:         stdin,
		stdinPiped:    stdinPiped,
		initialPrompt: initialPrompt,
		sessionID:     sessionID,
		forceResume:   forceResume,
	})
	return err
}

// OpenShellWidget launches the widget TUI and returns the final shell handoff command.
func OpenShellWidget(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	sessionID string,
	forceResume bool,
) (string, error) {
	initialDraft, _, err := readProgramStdin()
	if err != nil {
		return "", err
	}
	ttyRW, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening /dev/tty: %w", err)
	}
	defer func() { _ = ttyRW.Close() }()

	return openProgram(openProgramConfig{
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		cfgPath:       cfgPath,
		initialDraft:  initialDraft,
		sessionID:     sessionID,
		forceResume:   forceResume,
		programInput:  ttyRW,
		programOutput: ttyRW,
	})
}

func readProgramStdin() (string, bool, error) {
	text, piped, err := tty.ReadPiped(os.Stdin)
	if err != nil {
		return "", false, fmt.Errorf("reading stdin: %w", err)
	}
	return text, piped, nil
}

func openProgram(cfg openProgramConfig) (string, error) {
	// Determine program input. When stdin is piped we need to reopen the TTY
	// so bubbletea can receive keyboard events. Mouse capture (for viewport
	// wheel scrolling and selection) is requested per-frame via View.MouseMode
	// in v2, not as a program option.
	var programOpts []tea.ProgramOption
	outputFile := os.Stdout
	inputFile := os.Stdin

	if cfg.programInput != nil {
		inputFile = cfg.programInput
		programOpts = append(programOpts, tea.WithInput(cfg.programInput))
	}
	if cfg.programOutput != nil {
		outputFile = cfg.programOutput
		programOpts = append(programOpts, tea.WithOutput(cfg.programOutput))
	} else if cfg.stdinPiped {
		ttyFile, err := os.Open("/dev/tty")
		if err != nil {
			return "", fmt.Errorf("opening /dev/tty for keyboard input: %w", err)
		}
		defer func() { _ = ttyFile.Close() }()
		inputFile = ttyFile
		programOpts = append(programOpts, tea.WithInput(ttyFile))
	}

	// Queried before NewProgram so the DSR reply doesn't end up in bubbletea's
	// input queue. -1 = unknown; the model falls back to bottom-anchoring.
	initialCursorRow := -1
	if row, err := tty.QueryCursorRow(inputFile, outputFile); err == nil {
		initialCursorRow = row
	}

	// Detect the terminal background once, before the program starts, so the
	// light/dark palette and glamour theme are chosen correctly. Querying here
	// (like the cursor-row query above) keeps the OSC 11 reply out of
	// bubbletea's input queue.
	hasDarkBg := lipgloss.HasDarkBackground(inputFile, outputFile)

	settings := loadTUISettings(cfg.cfgPath, cfg.providerName)
	stores := openStores(cfg.cfgPath)

	// Load prior session if requested.
	var initialSession *history.Session
	if cfg.sessionID != "" && stores.sessions != nil && (settings.autoResume || cfg.forceResume) {
		if s, ok, err := stores.sessions.Load(cfg.sessionID); err == nil && ok {
			initialSession = &s
		}
	}

	env := envcontext.Detect(context.Background(), envcontext.Options{ShellHistory: settings.shellContext})
	system, toolDefs := cfg.mode.systemAndTools(env)
	m := newModel(modelConfig{
		providerName:     cfg.providerName,
		provider:         cfg.provider,
		modelID:          cfg.modelID,
		mode:             cfg.mode,
		system:           system,
		toolDefs:         toolDefs,
		llmJudge:         settings.llmJudge,
		suggestions:      settings.suggestions,
		effort:           settings.effort,
		themeName:        settings.themeName,
		closeKey:         settings.closeKey,
		hasDarkBg:        hasDarkBg,
		stdin:            cfg.stdin,
		initialDraft:     cfg.initialDraft,
		initialPrompt:    cfg.initialPrompt,
		initialSession:   initialSession,
		cfgPath:          cfg.cfgPath,
		sessionID:        cfg.sessionID,
		promptHistory:    stores.prompts,
		sessionStore:     stores.sessions,
		exchanges:        stores.exchanges,
		initialCursorRow: initialCursorRow,
	})

	// Surface any config warnings as a SystemEntry so they remain visible (and
	// terminal-selectable) inside the TUI — stderr text is wiped by the alt-screen.
	if cfg.cfgPath != "" {
		if warnings := config.CheckConfigWarnings(cfg.cfgPath); len(warnings) > 0 {
			m.appendThreadEntries(SystemEntry{
				Content: "Config warnings:\n" + strings.Join(warnings, "\n"),
			})
		}
		if notice := config.EnvOnlyNotice(cfg.cfgPath); notice != "" {
			m.appendThreadEntries(SystemEntry{Content: notice})
		}
	}

	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("TUI error: %w", err)
	}
	if model, ok := finalModel.(Model); ok {
		return model.exitShellCommand(), nil
	}
	return "", nil
}

// tuiSettings are the config-file settings the TUI reads at startup.
type tuiSettings struct {
	themeName    string
	llmJudge     bool
	shellContext bool
	closeKey     string
	autoResume   bool
	suggestions  bool
	effort       string
}

// loadTUISettings reads the startup settings from the config file. With no
// config path, or a config file that is missing or unreadable, the defaults
// stand: the default theme and suggestions on, everything else off or empty.
func loadTUISettings(cfgPath, providerName string) tuiSettings {
	s := tuiSettings{themeName: theme.DefaultName, suggestions: true}
	if cfgPath == "" {
		return s
	}
	cfgFile, exists, err := config.LoadConfig(cfgPath)
	if err != nil || !exists {
		return s
	}
	s.themeName = theme.Normalize(cfgFile.Settings.Theme)
	s.llmJudge = config.ResolveBoolSetting(cfgFile, "llm_judge")
	s.closeKey = cfgFile.Settings.Keybinding
	s.autoResume = config.ResolveBoolSetting(cfgFile, "auto_resume")
	s.suggestions = config.ResolveBoolSetting(cfgFile, "suggestions")
	s.shellContext = config.ResolveBoolSetting(cfgFile, "shell_context")
	s.effort = cfgFile.Providers[providerName].Effort
	return s
}

// tuiStores are the persistent prompt history, session store and exchange log,
// kept beside the config file. All nil when there is no config path.
type tuiStores struct {
	prompts   *history.PromptHistory
	sessions  *history.SessionStore
	exchanges *history.ExchangeLog
}

// openStores opens the persistent stores in the config file's directory,
// loading prompt history and pruning old sessions as it goes.
func openStores(cfgPath string) tuiStores {
	if cfgPath == "" {
		return tuiStores{}
	}
	configDir := filepath.Dir(cfgPath)

	prompts := history.NewPromptHistory(configDir)
	prompts.Load() //nolint:errcheck // Empty or unreadable history should not block the TUI.

	sessions := history.NewSessionStore(configDir)
	sessions.PruneOld()

	return tuiStores{
		prompts:   prompts,
		sessions:  sessions,
		exchanges: history.NewExchangeLog(configDir),
	}
}
