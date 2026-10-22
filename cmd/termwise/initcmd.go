package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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
				return fmt.Errorf("could not tell your shell from $SHELL — name it: termwise init --install %s", strings.Join(shellrc.Supported, "|"))
			}
			shell = detected
		} else {
			return fmt.Errorf("name a shell: termwise init %s (or termwise init --install)", strings.Join(shellrc.Supported, "|"))
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
			return fmt.Errorf("unsupported shell %q — supported: %s", shell, strings.Join(shellrc.Supported, ", "))
		}
		return nil
	},
}

func init() {
	initCmd.Flags().Bool("install", false, "Add the integration to your shell's startup file")
	initCmd.Flags().Bool("uninstall", false, "Remove the line --install added")
}
