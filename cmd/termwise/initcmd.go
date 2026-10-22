package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/shellrc"
)

var initCmd = &cobra.Command{
	Use:   "init [shell]",
	Short: "Output shell integration setup code",
	Long: `Output shell integration setup for your rc file.

Add to ~/.zshrc:
  eval "$(termwise init zsh)"

Add to ~/.bashrc:
  eval "$(termwise init bash)"

Add to ~/.config/fish/config.fish:
  termwise init fish | source

Or let termwise add the line for you with --install, which picks the shell
from $SHELL when none is given. --uninstall removes the line again.`,
	Example:       "  termwise init zsh\n  termwise init --install\n  termwise init --uninstall",
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		install, _ := cmd.Flags().GetBool("install")
		uninstall, _ := cmd.Flags().GetBool("uninstall")
		if install && uninstall {
			return fmt.Errorf("--install and --uninstall cannot be used together")
		}

		var shell string
		if len(args) == 1 {
			shell = args[0]
		} else if install || uninstall {
			detected, ok := shellrc.Detect()
			if !ok {
				return fmt.Errorf("could not tell your shell from $SHELL — name it: termwise init --install zsh|bash|fish")
			}
			shell = detected
		} else {
			return fmt.Errorf("name a shell: termwise init zsh|bash|fish (or termwise init --install)")
		}

		switch {
		case install:
			return installShellIntegration(shell)
		case uninstall:
			return uninstallShellIntegration(shell)
		}

		kb := readKeybinding()
		switch shell {
		case "zsh":
			fmt.Print(zshSetup(kb))
		case "bash":
			fmt.Print(bashSetup(kb))
		case "fish":
			fmt.Print(fishSetup(kb))
		default:
			return fmt.Errorf("unsupported shell %q — supported: zsh, bash, fish", shell)
		}
		return nil
	},
}

func init() {
	initCmd.Flags().Bool("install", false, "Add the integration to your shell's startup file")
	initCmd.Flags().Bool("uninstall", false, "Remove the line --install added")
}

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
	if _, err := exec.LookPath("termwise"); err == nil {
		return
	}
	dir := "the directory termwise is in"
	if exe, err := os.Executable(); err == nil {
		dir = config.ShortenHome(filepath.Dir(exe))
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
