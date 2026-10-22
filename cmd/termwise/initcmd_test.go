package main

import (
	"os/exec"
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

// The keybinding is pasted into a script the user's shell evals on start, so
// one with quote characters must not break its syntax: Alt+' is stored as \e'.
func TestShellSetupParsesWithQuotingKeybindings(t *testing.T) {
	for _, kb := range []string{"^T", `\e'`, `\e"`, `\e\`, `^\`} {
		for shell, setup := range map[string]string{"zsh": zshSetup(kb), "bash": bashSetup(kb)} {
			bin, err := exec.LookPath(shell)
			if err != nil {
				t.Logf("%s not installed; skipping", shell)
				continue
			}
			cmd := exec.Command(bin, "-n")
			cmd.Stdin = strings.NewReader(setup)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s setup for keybinding %q does not parse: %v\n%s", shell, kb, err, out)
			}
		}
	}
}

func TestShellSetupKeepsPlainBindingsUnchanged(t *testing.T) {
	if !strings.Contains(zshSetup("^T"), "\nbindkey '^T' _termwise_widget\n") {
		t.Error("zsh setup changed the bindkey line for ^T")
	}
	if !strings.Contains(bashSetup("^T"), `bind -x '"\C-t": _termwise_widget'`) {
		t.Error("bash setup changed the bind line for ^T")
	}
	if !strings.Contains(bashSetup(`\e"`), `bind -x '"\e\"": _termwise_widget'`) {
		t.Errorf("bash setup does not escape a quote in the key sequence:\n%s", bashSetup(`\e"`))
	}
}
