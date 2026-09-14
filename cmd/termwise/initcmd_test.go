package main

import (
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/cliname"
)

// Every shell the integration supports has to announce itself, or messages
// tell users with the integration to run `termwise config` instead of the
// shorter name they actually use.
func TestEveryShellSetupAnnouncesTheInvokedName(t *testing.T) {
	for shell, setup := range map[string]string{
		"zsh":  zshSetup("^T"),
		"bash": bashSetup("^T"),
		"fish": fishSetup("^T"),
	} {
		if !strings.Contains(setup, cliname.EnvVar) {
			t.Errorf("%s setup does not set %s", shell, cliname.EnvVar)
		}
		if !strings.Contains(setup, cliname.EnvVar+"=tw") && !strings.Contains(setup, cliname.EnvVar+" tw") {
			t.Errorf("%s setup sets %s to something other than tw", shell, cliname.EnvVar)
		}
	}
}
