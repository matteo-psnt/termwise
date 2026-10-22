package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/shellrc"
)

func installShellIntegration(shell string) error {
	path, err := shellrc.Path(shell)
	if err != nil {
		return err
	}
	added, err := shellrc.Install(path, shell)
	if err != nil {
		return fmt.Errorf("updating %s: %w", path, err)
	}
	if !added {
		fmt.Printf("%s already loads the termwise integration.\n", config.ShortenHome(path))
		return nil
	}
	fmt.Printf("Added to %s:\n  %s\n\nOpen a new terminal, or run `exec %s`, to get `tw` and %s.\n",
		config.ShortenHome(path), shellrc.Line(shell), shell, keybinding.Label(readKeybinding()))
	warnIfNotOnPath()
	return nil
}

func uninstallShellIntegration(shell string) error {
	path, err := shellrc.Path(shell)
	if err != nil {
		return err
	}
	removed, err := shellrc.Remove(path)
	if err != nil {
		return fmt.Errorf("updating %s: %w", path, err)
	}
	if removed {
		fmt.Printf("Removed the termwise integration from %s.\n", config.ShortenHome(path))
		return nil
	}
	if ok, _ := shellrc.Installed(path); ok {
		return fmt.Errorf("%s loads termwise with a line --install did not add — remove it by hand", config.ShortenHome(path))
	}
	fmt.Printf("%s does not load the termwise integration.\n", config.ShortenHome(path))
	return nil
}

// warnIfNotOnPath catches the go-install case: the rc line and the tw
// function both call `termwise` by name, so neither works until it resolves.
func warnIfNotOnPath() {
	ok, dir := termwiseOnPath()
	if ok {
		return
	}
	if dir == "" {
		dir = "the directory termwise is in"
	}
	fmt.Fprintf(os.Stderr, "\nwarning: termwise is not on your PATH, so the line above will fail.\n"+
		"  Add %s to PATH in your shell's startup file.\n", dir)
}

// readKeybinding returns the configured keybinding, or the default when
// there is none — including when the config file is missing or unreadable,
// since the shell integration should still load.
func readKeybinding() string {
	_, cfg, _, err := loadConfigFile()
	if err != nil || cfg.Settings.Keybinding == "" {
		return config.DefaultKeybinding
	}
	return cfg.Settings.Keybinding
}

// termwiseOnPath reports whether `termwise` resolves on PATH. When it does
// not, dir is the directory the running binary is in, shortened for display,
// or "" when that cannot be told.
func termwiseOnPath() (ok bool, dir string) {
	if _, err := exec.LookPath("termwise"); err == nil {
		return true, ""
	}
	if exe, err := os.Executable(); err == nil {
		return false, config.ShortenHome(filepath.Dir(exe))
	}
	return false, ""
}

// shellIntegration is how far the shell integration is set up for the user.
type shellIntegration int

const (
	// integrationLoaded: this shell has already loaded the integration.
	integrationLoaded shellIntegration = iota
	// integrationUnsupported: $SHELL is not a shell termwise supports.
	integrationUnsupported
	// integrationNoRCFile: the shell's startup file cannot be located.
	integrationNoRCFile
	// integrationInstalled: the startup file loads it, but this shell has not.
	integrationInstalled
	// integrationNotInstalled: the startup file does not load it.
	integrationNotInstalled
)

// detectShellIntegration works out how far the integration is set up, and
// for the last two states, which shell and startup file it is about.
func detectShellIntegration() (state shellIntegration, shell, rcPath string) {
	if os.Getenv(cliname.EnvVar) != "" {
		return integrationLoaded, "", ""
	}
	shell, ok := shellrc.Detect()
	if !ok {
		return integrationUnsupported, "", ""
	}
	rcPath, err := shellrc.Path(shell)
	if err != nil {
		return integrationNoRCFile, shell, ""
	}
	if installed, _ := shellrc.Installed(rcPath); installed {
		return integrationInstalled, shell, rcPath
	}
	return integrationNotInstalled, shell, rcPath
}
