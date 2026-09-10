package agentui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// /theme was typed five times in real use and every one of those was sent to
// the model as an ordinary prompt, because only /effort, /model, /config,
// /clear and /help were registered.
func TestThemeIsARegisteredSlashCommand(t *testing.T) {
	cmd := findSlashCommand("/theme")
	if cmd == nil {
		t.Fatal("/theme is not registered")
	}
	if cmd.Run == nil || cmd.Preview == nil || cmd.ArgOptions == nil {
		t.Error("/theme should have a runner, a live preview, and arg completion like /model does")
	}
	if len(cmd.ArgOptions(Model{})) == 0 {
		t.Error("/theme offers no theme names for completion")
	}
}

// Every setting that has its own slash command should complete to real values.
func TestThemeArgOptionsAreValidThemes(t *testing.T) {
	for _, name := range themeArgOptions(Model{}) {
		if !theme.IsValid(name) {
			t.Errorf("%q is offered for completion but is not a valid theme", name)
		}
	}
}

func TestUnknownSlashCommandStillFallsThrough(t *testing.T) {
	if findSlashCommand("/definitely-not-a-command") != nil {
		t.Error("unknown command unexpectedly resolved")
	}
}
