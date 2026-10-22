package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/tui/configui"
)

var configCmd = &cobra.Command{
	Use:           "config",
	Short:         "Manage termwise configuration",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		if cfgPath, err := config.DefaultConfigPath(); err == nil {
			emitConfigWarnings(cfgPath)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Any positional args mean cobra didn't match a subcommand — show help
		// rather than trying to open the TUI with garbage input.
		if len(args) > 0 {
			return cmd.Help()
		}
		if edit, _ := cmd.Flags().GetBool("edit"); edit {
			return editConfig()
		}
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		cfg, exists, err := config.LoadConfig(cfgPath)
		if err != nil {
			// Without a terminal there is no one to work an editor, and
			// starting one would hang a script or CI job: say what is wrong
			// and how to fix it instead.
			if !interactive() {
				return fmt.Errorf("%w\n  %s", err, repairHint())
			}
			// Config exists but failed to parse — open it in $EDITOR so the
			// user can repair it directly. The TUI is recovery, not a dead end.
			fmt.Fprintf(os.Stderr, "Warning: %s\n\nOpening file for repair — fix it and save, or delete it to start fresh.\n\n", err)
			return editConfig()
		}
		if !interactive() {
			return fmt.Errorf("the setup screen needs a terminal — from a script, use `%[1]s config use <provider> [--model …]` (see `%[1]s config use --help`)",
				cliname.Name())
		}
		if err := configui.Open(cfgPath, cfg, exists); err != nil {
			return err
		}
		if !exists && config.ConfigExists(cfgPath) {
			offerShellIntegration(os.Stdin, os.Stdout)
		}
		return nil
	},
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadConfigFile loads the config file at the default path; a missing file
// loads as an empty config. A file that cannot be loaded comes back as an
// error with the recovery hint attached, so the user knows how to fix it
// before retrying.
func loadConfigFile() (cfgPath string, cfg config.FileConfig, err error) {
	cfgPath, err = config.DefaultConfigPath()
	if err != nil {
		return "", config.FileConfig{}, err
	}
	cfg, _, err = config.LoadConfig(cfgPath)
	if err != nil {
		return cfgPath, config.FileConfig{}, fmt.Errorf("%w\n  %s", err, repairHint())
	}
	return cfgPath, cfg, nil
}

// repairHint is the next step for a config file that does not load.
// LoadConfig's error already says the file is invalid and where it is.
func repairHint() string {
	return fmt.Sprintf("Run `%s config --edit` to repair it", cliname.Name())
}

// mutateConfig loads the config, applies fn, and saves atomically, holding
// the config lock so a save from the setup screen or another command cannot
// land in between and be lost.
func mutateConfig(fn func(*config.FileConfig) error) error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	err = config.MutateConfig(cfgPath, fn)
	var le *config.LoadError
	if errors.As(err, &le) {
		return fmt.Errorf("%w\n  %s", err, repairHint())
	}
	return err
}

// mutateActiveProvider is mutateConfig scoped to the currently selected provider.
func mutateActiveProvider(fn func(*config.FileConfig, string) error) error {
	return mutateConfig(func(cfg *config.FileConfig) error {
		name := cfg.SelectedProvider
		if name == "" {
			return fmt.Errorf("no provider selected — run `%s config use <provider>` first", cliname.Name())
		}
		return fn(cfg, name)
	})
}

// editConfig opens the config file in $EDITOR.
func editConfig() error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := config.SaveConfig(cfgPath, config.FileConfig{}); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		for _, e := range []string{"vim", "nano", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found — set $EDITOR")
	}

	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Validate after editing.
	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
		return nil
	}
	for _, e := range append(config.Validate(cfg), config.EffortProblems(cfg)...) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", e)
	}
	return nil
}

func init() {
	configCmd.Flags().Bool("edit", false, "Open config file in $EDITOR")

	configModelCmd.AddCommand(configModelListCmd)
	configModelCmd.AddCommand(configModelSetCmd)

	configAuthCmd.AddCommand(configAuthEnvCmd)
	configAuthCmd.AddCommand(configAuthCmdCmd)
	configAuthCmd.AddCommand(configAuthKeychainCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configUseCmd)
	configCmd.AddCommand(configBaseURLCmd)
	configCmd.AddCommand(configModelCmd)
	configCmd.AddCommand(configAuthCmd)
	configCmd.AddCommand(configTestCmd)
	configCmd.AddCommand(configDoctorCmd)
	configCmd.AddCommand(configRemoveCmd)
}
