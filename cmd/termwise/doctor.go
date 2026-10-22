package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// ---------------------------------------------------------------------------
// tw config doctor
// ---------------------------------------------------------------------------

var configDoctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "Check configuration, provider connection and shell setup",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		offline, _ := cmd.Flags().GetBool("offline")
		if !runDoctor(cfgPath, !offline) {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	configDoctorCmd.Flags().Bool("offline", false, "Skip the check that contacts the provider")
}

// runDoctor prints a per-check diagnostic report and returns true if all pass.
// online adds a live request to the provider. Warnings point at something
// worth knowing but do not fail the run.
func runDoctor(cfgPath string, online bool) bool {
	allOk := true
	pass := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	fail := func(msg string) { fmt.Printf("  ✗ %s\n", msg); allOk = false }
	warn := func(msg string) { fmt.Printf("  ! %s\n", msg) }
	hint := func(format string, a ...any) { fmt.Printf("    → "+format+"\n", a...) }

	fmt.Printf("Config: %s\n\n", cfgPath)

	// Check what the runtime would run with, not just the file: with no file
	// it falls back to an API key in the environment, and TERMWISE_PROVIDER /
	// TERMWISE_MODEL override the file for the run.
	if _, _, err := config.LoadConfig(cfgPath); err != nil {
		fail("File: " + err.Error())
		hint("%s", repairHint())
		return false
	}
	eff, err := config.LoadEffective(cfgPath)
	switch {
	case errors.Is(err, config.ErrNoConfig):
		fail("File: not found, and no provider API key in the environment")
		hint("Run `%s config` to set up", cliname.Name())
		return false
	case err != nil:
		// The file loaded, so this is a bad TERMWISE_* override.
		fail(err.Error())
		return false
	}
	cfg := eff.File
	if eff.Exists {
		pass("File: exists and valid TOML")
	} else {
		pass("File: none — running from the environment")
		hint("Run `%s config` to save a config", cliname.Name())
	}
	for _, o := range eff.Overrides {
		warn("Override: " + o + " (from the environment, not the file)")
	}

	// selected_provider set and known.
	if cfg.SelectedProvider == "" {
		fail("selected_provider: not set")
		hint("Run `%s config use <provider>`", cliname.Name())
		return false
	}
	knownNames := provider.Names()
	known := slices.Contains(knownNames, cfg.SelectedProvider)
	if known {
		pass("selected_provider: " + cfg.SelectedProvider)
	} else {
		fail("selected_provider: " + cfg.SelectedProvider + " (not a known provider)")
		hint("Supported: %s", strings.Join(knownNames, ", "))
	}

	// Provider block.
	pc, hasBlock := cfg.Providers[cfg.SelectedProvider]
	if !hasBlock {
		fail("[providers." + cfg.SelectedProvider + "]: no config block")
		hint("Run `%s config` to configure it", cliname.Name())
		return allOk
	}
	pass("[providers." + cfg.SelectedProvider + "]: block found")

	// Model.
	if pc.Model != "" {
		pass("Model: " + pc.Model)
	} else {
		fail("Model: not set")
		hint("Run `%s config model set <model>`", cliname.Name())
	}

	// Auth credentials.
	authOk := true
	if provider.HasNoAuth(cfg.SelectedProvider) {
		pass("Auth: not required")
	} else {
		authDesc := authSummary(cfg.SelectedProvider, pc)
		auth, authErr := config.ResolveAuth(cfg.SelectedProvider, pc)
		switch {
		case authErr == nil && auth.FallbackErr != nil:
			// ResolveAuth fell back to the provider's default env var: it works
			// today, but not the way the config says.
			warn("Auth: " + authDesc + " failed, using " + auth.Source + " instead")
			hint("%s", strings.ReplaceAll(auth.FallbackErr.Error(), "\n", "\n      "))
		case authErr == nil && auth.Source == "":
			pass("Auth: no key needed")
		case authErr == nil:
			pass("Auth: key from " + auth.Source)
		default:
			fail("Auth: " + authDesc + " — " + authErr.Error())
			authOk = false
		}
	}

	// Connection: the only check that proves the key and URL actually work.
	switch {
	case !online:
		warn("Connection: skipped (--offline)")
	case !known || !authOk:
		warn("Connection: skipped until the problems above are fixed")
	default:
		ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
		err := config.CheckProviderConnectivity(ctx, cfg.SelectedProvider, pc)
		cancel()
		if err == nil {
			pass("Connection: " + cfg.SelectedProvider + " answered")
		} else {
			fail("Connection: " + strings.ReplaceAll(err.Error(), "\n", "\n    "))
		}
	}

	doctorShell(pass, warn, hint, cfg)

	// Settings — validated through the central registry so each SettingDef owns its rules.
	for _, def := range config.Settings {
		val := def.Get(cfg)
		display := def.Resolve(cfg)
		label := def.Label
		if def.Validate != nil {
			if err := def.Validate(val); err != nil {
				fail(label + ": " + err.Error())
				continue
			}
		}
		if val == "" {
			pass(label + ": " + display + " (default)")
		} else {
			pass(label + ": " + display)
		}
	}

	fmt.Println()
	if allOk {
		fmt.Println("All checks passed.")
	}
	return allOk
}

// doctorShell checks the pieces outside the config file that `tw` and the
// keybinding depend on. Everything here is a warning: termwise itself works
// without the integration.
func doctorShell(pass, warn func(string), hint func(string, ...any), cfg config.FileConfig) {
	if ok, dir := termwiseOnPath(); ok {
		pass("PATH: termwise found")
	} else {
		warn("PATH: termwise is not on your PATH, so `tw` and the shell integration cannot run it")
		if dir != "" {
			hint("Add %s to PATH in your shell's startup file", dir)
		}
	}

	state, shell, path := detectShellIntegration()
	switch state {
	case integrationLoaded:
		pass("Shell integration: loaded in this shell")
	case integrationUnsupported:
		warn("Shell integration: not loaded, and $SHELL is not zsh, bash or fish")
	case integrationInstalled:
		warn("Shell integration: in " + config.ShortenHome(path) + " but not loaded in this shell")
		hint("Open a new terminal, or run `exec %s`", shell)
	case integrationNotInstalled:
		warn("Shell integration: not installed — no `tw` command or keybinding")
		hint("Run `termwise init --install`")
	}

	if kb := cfg.Settings.Keybinding; kb == "" || kb == config.DefaultKeybinding {
		if _, err := exec.LookPath("fzf"); err == nil {
			warn("Keybinding: fzf is installed, and its shell keybindings also use Ctrl+T — whichever loads last wins")
			hint("Pick another key in `%s config` → Keybinding", cliname.Name())
		}
	}
}
