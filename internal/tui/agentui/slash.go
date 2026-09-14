package agentui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/matteo-psnt/termwise/internal/cliname"

	tea "charm.land/bubbletea/v2"

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
	// Preview applies a candidate value as in-memory state only. It is
	// called by the slash picker on every cursor move so the user sees the
	// effect live, and again with the original value on cancel to revert.
	// nil means the command has no live-preview behavior.
	Preview func(m Model, value string) Model
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
			Preview:     previewEffort,
		},
		{
			Name:        "/model",
			Description: "switch active model",
			ArgOptions:  modelArgOptions,
			Run:         runModel,
			Preview:     previewModel,
		},
		{
			Name:        "/theme",
			Description: "switch color theme",
			ArgOptions:  themeArgOptions,
			Run:         runTheme,
			Preview:     previewTheme,
		},
		{
			Name:        "/config",
			Description: "edit settings",
			Run:         runConfig,
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
	m.appendThreadEntries(UserEntry{Content: text})
	return cmd.Run(m, parts[1:])
}

// --- /theme ----------------------------------------------------------------

func themeArgOptions(_ Model) []string { return theme.Names() }

// runTheme opens the theme picker, or applies a named theme directly.
//
// Theme is the most visible setting there is, and it sits behind /config
// alongside everything else — so people reach for /theme, which used to fall
// through to the model as an ordinary prompt.
func runTheme(m Model, args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.openSlashPicker("/theme", "Theme", "Color scheme for the interface.",
			simpleOptions(themeArgOptions(m)), m.themeName)
	}
	name := theme.Normalize(args[0])
	if !theme.IsValid(name) {
		m.appendThreadEntries(ErrorEntry{
			Content: "Unknown theme '" + args[0] + "' — valid: " + strings.Join(theme.Names(), ", "),
		})
		m.refreshViewport()
		return m, nil
	}
	m = previewTheme(m, name)
	if def, ok := config.FindSetting("theme"); ok {
		m.persistConfigValue(def, name)
	}
	m.appendThreadEntries(SystemEntry{Content: "Theme set to " + name + "."})
	m.refreshViewport()
	return m, nil
}

// previewTheme applies a theme in memory only, so moving the picker cursor
// repaints the whole interface live and cancelling puts the old one back.
func previewTheme(m Model, value string) Model {
	if name := theme.Normalize(value); theme.IsValid(name) {
		m.applyTheme(name)
	}
	return m
}

// --- /effort ---------------------------------------------------------------

func effortArgOptions(_ Model) []string {
	return []string{"low", "medium", "high"}
}

func runEffort(m Model, args []string) (tea.Model, tea.Cmd) {
	if !models.SupportsThinking(m.providerName, m.modelID) {
		name := m.modelShortName()
		m.appendThreadEntries(ErrorEntry{
			Content: "Reasoning effort isn't supported by " + name + " — pick a thinking-capable model with /model.",
		})
		m.refreshViewport()
		return m, nil
	}
	if len(args) == 0 {
		return m.openSlashPicker("/effort", "Reasoning effort", "How much the model thinks before responding.", simpleOptions(effortArgOptions(m)), m.effort)
	}
	level := normalizeEffortInput(args[0])
	if level == "" {
		m.appendThreadEntries(ErrorEntry{Content: "Unknown effort level '" + args[0] + "' — valid: low, med, high"})
		m.refreshViewport()
		return m, nil
	}
	m = previewEffort(m, level)
	if m.cfgPath != "" {
		cfg, exists, err := config.LoadConfig(m.cfgPath)
		if err == nil && exists {
			pc := cfg.Providers[m.providerName]
			pc.Effort = m.effort
			cfg.Providers[m.providerName] = pc
			_ = config.SaveConfig(m.cfgPath, cfg)
		}
	}
	m.appendThreadEntries(SystemEntry{Content: "Effort set to " + level + "."})
	m.refreshViewport()
	return m, nil
}

// previewEffort applies an effort level in memory only. Called by the picker
// on cursor move, and again with the prior value on cancel to revert. An empty
// or unrecognized value is treated as the default (stored as "").
func previewEffort(m Model, value string) Model {
	if !models.SupportsThinking(m.providerName, m.modelID) {
		return m
	}
	level := normalizeEffortInput(value)
	stored := level
	if level == config.DefaultEffort {
		stored = ""
	}
	m.effort = stored
	return m
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

// configuredModelOptions returns one slashOption per (provider, model) pair
// across all configured providers. Label is the human model name, Detail is
// the provider name, Value is the canonical "provider/modelID" string. Returns
// nil when no config is available or no providers are configured. The first
// option for the currently active provider is sorted to the top to make
// switching within a provider one keystroke.
func configuredModelOptions(m Model) []slashOption {
	if m.cfgPath == "" {
		return nil
	}
	cfg, exists, err := config.LoadConfig(m.cfgPath)
	if err != nil || !exists || len(cfg.Providers) == 0 {
		return nil
	}

	providerOrder := make([]string, 0, len(cfg.Providers))
	if _, ok := cfg.Providers[m.providerName]; ok {
		providerOrder = append(providerOrder, m.providerName)
	}
	others := make([]string, 0, len(cfg.Providers))
	for p := range cfg.Providers {
		if p != m.providerName {
			others = append(others, p)
		}
	}
	sort.Strings(others)
	providerOrder = append(providerOrder, others...)

	var out []slashOption
	for _, p := range providerOrder {
		pd, ok := models.Provider(p)
		if !ok {
			continue
		}
		for _, md := range pd.Models {
			label := md.Name
			if label == "" {
				label = md.ID
			}
			out = append(out, slashOption{
				Label:  label,
				Detail: p,
				Value:  modelOptionFor(p, md.ID),
			})
		}
	}
	return out
}

func modelArgOptions(m Model) []string {
	opts := configuredModelOptions(m)
	if len(opts) == 0 {
		return nil
	}
	out := make([]string, len(opts))
	for i, opt := range opts {
		out[i] = opt.Value
	}
	return out
}

func runModel(m Model, args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		opts := configuredModelOptions(m)
		if len(opts) == 0 {
			m.appendThreadEntries(ErrorEntry{Content: fmt.Sprintf("No providers configured — run `%s config` to add one.", cliname.Name())})
			m.refreshViewport()
			return m, nil
		}
		return m.openSlashPicker("/model", "Active model", "Switch the model used for this session.", opts, modelOptionFor(m.providerName, m.modelID))
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
	if err := m.applyModel(providerName, modelID); err != nil {
		m.appendThreadEntries(ErrorEntry{Content: err.Error()})
		m.refreshViewport()
		return m, nil
	}
	m.persistActiveModel()
	m.refreshViewport()
	return m, nil
}

// previewModel updates the displayed provider+model in memory only. The actual
// provider client is not rebuilt — that happens on submit via runModel → applyModel —
// keeping cursor-move latency off the LoadConfig/ResolveAuth/NewClient hot path.
// Errors fall through silently; runModel re-validates on submit.
func previewModel(m Model, value string) Model {
	providerName, modelID, ok := strings.Cut(value, "/")
	if !ok || providerName == "" || modelID == "" {
		return m
	}
	md := models.Find(providerName, modelID)
	if md == nil {
		return m
	}
	m.providerName = providerName
	m.modelID = modelID
	m.contextWindow = 32_000
	if md.Context > 0 {
		m.contextWindow = md.Context
	}
	return m
}

// --- /config ---------------------------------------------------------------

func runConfig(m Model, _ []string) (tea.Model, tea.Cmd) {
	return m.openConfigEditor(), nil
}

// --- /clear ----------------------------------------------------------------

func runClear(m Model, _ []string) (tea.Model, tea.Cmd) {
	m.thread = nil
	m.messages = nil
	m.stdin = ""
	m.inputTokens = 0
	m.outputTokens = 0
	m.histIdx = -1
	m.histDraft = ""
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
	width := 0
	for _, c := range slashCommands {
		if w := len(c.Name); w > width {
			width = w
		}
	}

	var b strings.Builder
	b.WriteString("Slash commands:\n")
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
