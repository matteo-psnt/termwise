package agentui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// slashCommand describes a single user-visible slash command.
type slashCommand struct {
	Name        string
	Description string
	// ArgOptions returns the valid argument completions for this command,
	// or nil if the command takes no completable arguments.
	ArgOptions func(m Model) []string
	// Run is invoked when the user submits the command. args is the
	// whitespace-split tokens after the command name (possibly empty).
	Run func(m Model, args []string) (tea.Model, tea.Cmd)
}

// slashCommands is the registry of built-in slash commands, in dropdown order.
// Populated in init() to break the cycle between runHelp and slashCommands.
var slashCommands []slashCommand

func init() {
	slashCommands = []slashCommand{
		{
			Name:        "/effort",
			Description: "set reasoning effort",
			ArgOptions:  effortArgOptions,
			Run:         runEffort,
		},
		{
			Name:        "/model",
			Description: "switch active model",
			ArgOptions:  modelArgOptions,
			Run:         runModel,
		},
		{
			Name:        "/theme",
			Description: "switch UI theme",
			ArgOptions:  themeArgOptions,
			Run:         runTheme,
		},
		{
			Name:        "/clear",
			Description: "clear conversation",
			Run:         runClear,
		},
		{
			Name:        "/help",
			Description: "list slash commands",
			Run:         runHelp,
		},
	}
}

// findSlashCommand returns the command with the given name, or nil.
func findSlashCommand(name string) *slashCommand {
	for i := range slashCommands {
		if slashCommands[i].Name == name {
			return &slashCommands[i]
		}
	}
	return nil
}

// slashMatch is one row in the slash-command dropdown. Description is empty
// for arg matches.
type slashMatch struct {
	Label       string
	Description string
	// Completion is the suffix to append to the current input value to fill
	// in the highlighted match (used for inline ghost text).
	Completion string
}

// computeSlashMatches returns the dropdown matches for the given input value.
// Returns nil when the value does not start with "/" or when there are no
// matches. The boolean `argMode` reports whether we're completing arg tokens
// (as opposed to the command name).
func computeSlashMatches(m Model, value string) (matches []slashMatch, argMode bool) {
	if !strings.HasPrefix(value, "/") {
		return nil, false
	}
	if name, argPrefix, ok := strings.Cut(value, " "); ok {
		cmd := findSlashCommand(name)
		if cmd == nil || cmd.ArgOptions == nil {
			return nil, true
		}
		for _, opt := range cmd.ArgOptions(m) {
			if strings.HasPrefix(opt, argPrefix) {
				matches = append(matches, slashMatch{
					Label:      opt,
					Completion: opt[len(argPrefix):],
				})
			}
		}
		return matches, true
	}
	for _, c := range slashCommands {
		if strings.HasPrefix(c.Name, value) {
			matches = append(matches, slashMatch{
				Label:       c.Name,
				Description: c.Description,
				Completion:  c.Name[len(value):],
			})
		}
	}
	return matches, false
}

// dispatchSlashCommand parses `text` and runs the matching command.
// Unknown commands fall back to sending `text` as a normal message.
func (m Model) dispatchSlashCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return m.submitMessage(text)
	}
	cmd := findSlashCommand(parts[0])
	if cmd == nil {
		return m.submitMessage(text)
	}
	return cmd.Run(m, parts[1:])
}

// --- /effort ---------------------------------------------------------------

func effortArgOptions(_ Model) []string {
	return []string{"low", "medium", "high"}
}

func runEffort(m Model, args []string) (tea.Model, tea.Cmd) {
	if !models.SupportsThinking(m.providerName, m.modelID) {
		m.appendThreadEntries(ErrorEntry{Content: "Current model doesn't support reasoning effort"})
		m.refreshViewport()
		return m, nil
	}
	if len(args) == 0 {
		return m.openSlashPicker("/effort", "Reasoning effort", effortArgOptions(m), m.effort)
	}
	level := normalizeEffortInput(args[0])
	if level == "" {
		m.appendThreadEntries(ErrorEntry{Content: "Unknown effort level '" + args[0] + "' — valid: low, med, high"})
		m.refreshViewport()
		return m, nil
	}
	m.effort = level
	if m.cfgPath != "" {
		cfg, exists, err := config.LoadConfig(m.cfgPath)
		if err == nil && exists {
			cfg.Settings.Effort = level
			_ = config.SaveConfig(m.cfgPath, cfg)
		}
	}
	m.refreshViewport()
	return m, nil
}

func normalizeEffortInput(s string) string {
	switch strings.ToLower(s) {
	case "low":
		return "low"
	case "med", "medium":
		return "medium"
	case "high":
		return "high"
	}
	return ""
}

// --- /model ----------------------------------------------------------------

// modelOptionFor returns the canonical /model arg for a given provider+model,
// formatted as "provider/modelID".
func modelOptionFor(providerName, modelID string) string {
	return providerName + "/" + modelID
}

func modelArgOptions(_ Model) []string {
	var out []string
	for _, p := range models.KnownProviders() {
		pd, ok := models.Provider(p)
		if !ok {
			continue
		}
		for _, md := range pd.Models {
			out = append(out, modelOptionFor(p, md.ID))
		}
	}
	return out
}

func runModel(m Model, args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.openSlashPicker("/model", "Active model", modelArgOptions(m), modelOptionFor(m.providerName, m.modelID))
	}
	providerName, modelID, ok := strings.Cut(args[0], "/")
	if !ok || providerName == "" || modelID == "" {
		m.appendThreadEntries(ErrorEntry{Content: "Usage: /model provider/model-id (e.g. anthropic/claude-sonnet-4-5)"})
		m.refreshViewport()
		return m, nil
	}
	if models.Find(providerName, modelID) == nil {
		m.appendThreadEntries(ErrorEntry{Content: "Unknown model '" + args[0] + "'"})
		m.refreshViewport()
		return m, nil
	}
	if err := m.switchModel(providerName, modelID); err != nil {
		m.appendThreadEntries(ErrorEntry{Content: err.Error()})
		m.refreshViewport()
		return m, nil
	}
	m.refreshViewport()
	return m, nil
}

// --- /theme ----------------------------------------------------------------

func themeArgOptions(_ Model) []string {
	return theme.Names()
}

func runTheme(m Model, args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.openSlashPicker("/theme", "UI theme", theme.Names(), m.themeName)
	}
	name := theme.Normalize(args[0])
	if !theme.IsValid(name) {
		m.appendThreadEntries(ErrorEntry{Content: "Unknown theme '" + args[0] + "'"})
		m.refreshViewport()
		return m, nil
	}
	m.applyTheme(name)
	if m.cfgPath != "" {
		cfg, exists, err := config.LoadConfig(m.cfgPath)
		if err == nil && exists {
			cfg.Settings.Theme = name
			_ = config.SaveConfig(m.cfgPath, cfg)
		}
	}
	m.refreshViewport()
	return m, nil
}

// --- /clear ----------------------------------------------------------------

func runClear(m Model, _ []string) (tea.Model, tea.Cmd) {
	m.thread = nil
	m.messages = nil
	m.stdin = ""
	m.inputTokens = 0
	m.outputTokens = 0
	m.clearSuggestion()
	m.clearShellCommand()
	if m.sessionStore != nil && m.sessionID != "" {
		_ = m.sessionStore.Delete(m.sessionID)
	}
	m.refreshViewport()
	return m, nil
}

// --- /help -----------------------------------------------------------------

func runHelp(m Model, _ []string) (tea.Model, tea.Cmd) {
	var b strings.Builder
	b.WriteString("Slash commands:\n")
	width := 0
	for _, c := range slashCommands {
		if w := len(c.Name); w > width {
			width = w
		}
	}
	for i, c := range slashCommands {
		b.WriteString("  " + c.Name + strings.Repeat(" ", width-len(c.Name)+2) + c.Description)
		if i < len(slashCommands)-1 {
			b.WriteString("\n")
		}
	}
	m.appendThreadEntries(SystemEntry{Content: b.String()})
	m.refreshViewport()
	return m, nil
}
