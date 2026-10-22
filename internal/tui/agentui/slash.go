package agentui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
	// Replace is how many bytes at the end of the input Completion replaces
	// rather than follows. Zero for everything but an @ path that has to be
	// rewritten into its quoted form.
	Replace int
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
