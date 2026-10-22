package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/shellrc"
	"github.com/matteo-psnt/termwise/internal/tty"
	"github.com/matteo-psnt/termwise/internal/tui/configui"
)

// resolveTUIRuntimeOrSetup is resolveTUIRuntime for entry points a person is
// sitting at: with nothing configured it runs setup and carries on, rather
// than exiting with instructions to go and run setup.
func resolveTUIRuntimeOrSetup() (tuiRuntime, error) {
	rt, err := resolveTUIRuntime()
	if !errors.Is(err, config.ErrNoConfig) || !interactive() {
		return rt, err
	}
	cfgPath, perr := config.DefaultConfigPath()
	if perr != nil {
		return rt, err
	}
	if serr := configui.Open(cfgPath, config.FileConfig{}, false); serr != nil {
		return tuiRuntime{}, serr
	}
	if !config.ConfigExists(cfgPath) {
		return tuiRuntime{}, fmt.Errorf("setup was not finished — run `%s config` when you're ready", cliname.Name())
	}
	offerShellIntegration(os.Stdin, os.Stdout)
	return resolveTUIRuntime()
}

func interactive() bool {
	return tty.IsTerminal(os.Stdin) && tty.IsTerminal(os.Stdout)
}

// offerShellIntegration asks to add the integration to the user's rc file
// once setup has produced a config. It stays quiet when the integration is
// already loaded or already in the file, and when it cannot tell the shell.
func offerShellIntegration(in io.Reader, out io.Writer) {
	state, shell, path := detectShellIntegration()
	if state == integrationInstalled {
		_, _ = fmt.Fprintf(out, "Open a new terminal to get `tw` and %s — %s already loads them.\n",
			keybinding.Label(readKeybinding()), config.ShortenHome(path))
		return
	}
	if state != integrationNotInstalled {
		return
	}

	_, _ = fmt.Fprintf(out, "\nAdd the `tw` command and the %s shortcut to %s? [Y/n] ",
		keybinding.Label(readKeybinding()), config.ShortenHome(path))
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		if err := installShellIntegration(shell); err != nil {
			_, _ = fmt.Fprintf(out, "Could not update %s: %v\n", config.ShortenHome(path), err)
		}
	default:
		_, _ = fmt.Fprintf(out, "Skipped. To add it later, run `termwise init --install`, or add this line to %s:\n  %s\n",
			config.ShortenHome(path), shellrc.Line(shell))
	}
}
