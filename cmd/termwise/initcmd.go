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

// readKeybinding returns the configured keybinding, defaulting to "^T".
func readKeybinding() string {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return "^T"
	}
	cfg, exists, err := config.LoadConfig(cfgPath)
	if err != nil || !exists {
		return "^T"
	}
	if cfg.Settings.Keybinding != "" {
		return cfg.Settings.Keybinding
	}
	return "^T"
}

func zshSetup(binding string) string {
	return fmt.Sprintf(`# termwise shell integration
export TERMWISE_INVOKED_AS=tw
tw() {
    if [[ $# -gt 0 ]]; then
        case "$1" in
            ask)
                command termwise "$@"
                return
                ;;
            explain)
                shift
                command termwise explain --session-id "$$" "$@"
                return
                ;;
            config|init|help|--help|-h|completion|__complete|__completeNoDesc)
                command termwise "$@"
                return
                ;;
        esac
        command termwise --session-id "$$" "$@"
    else
        command termwise --session-id "$$"
    fi
}

_termwise_widget() {
    local _tw_result _tw_status
    zle -I
    _tw_result="$(printf '%%s' "$BUFFER" | command termwise --shell-widget --session-id "$$")"
    _tw_status=$?
    if [[ $_tw_status -ne 0 ]]; then
        zle reset-prompt
        return
    fi
    if [[ -n "$_tw_result" ]]; then
        BUFFER="$_tw_result"
        CURSOR=${#BUFFER}
    fi
    zle reset-prompt
}
zle -N _termwise_widget
# Complete tw like termwise, when completion is set up. The tw function passes
# __complete through, which is what the completion script calls.
if (( $+functions[compdef] )); then
    compdef tw=termwise
fi
# Remove any previous _termwise_widget binding before applying the configured one.
while IFS= read -r _tw_key; do
    bindkey -r "$_tw_key"
done < <(bindkey | awk '/_termwise_widget$/{gsub(/"/, "", $1); print $1}')
unset _tw_key
bindkey '%s' _termwise_widget
`, binding)
}

func bashSetup(binding string) string {
	bashKey := keybinding.ToBash(binding)
	return fmt.Sprintf(`# termwise shell integration
export TERMWISE_INVOKED_AS=tw
tw() {
    case "$1" in
        ask|config|init|help|--help|-h|completion|__complete|__completeNoDesc)
            command termwise "$@"
            return
            ;;
        explain)
            shift
            command termwise explain --session-id "$$" "$@"
            return
            ;;
    esac
    if [[ $# -gt 0 ]]; then
        command termwise --session-id "$$" "$@"
    else
        command termwise --session-id "$$"
    fi
}

_termwise_widget() {
    local _tw_result _tw_status
    _tw_result="$(printf '%%s' "$READLINE_LINE" | command termwise --shell-widget --session-id "$$")"
    _tw_status=$?
    if [[ $_tw_status -ne 0 ]]; then
        return
    fi
    if [[ -n "$_tw_result" ]]; then
        READLINE_LINE="$_tw_result"
        READLINE_POINT=${#READLINE_LINE}
    fi
}
# Complete tw like termwise. bash-completion loads scripts lazily, so ask it
# for termwise's now; without bash-completion this is a no-op.
if declare -F _completion_loader >/dev/null; then
    _completion_loader termwise 2>/dev/null
fi
if declare -F __start_termwise >/dev/null; then
    complete -o default -F __start_termwise tw
fi

# Remove any previous _termwise_widget binding before applying the configured one.
while IFS= read -r _tw_key; do
    bind -r "\"$_tw_key\"" 2>/dev/null
done < <(bind -X 2>/dev/null | awk '/_termwise_widget/{gsub(/^""|"": .*/, "", $0); print}')
unset _tw_key
bind -x '"%s": _termwise_widget'
`, bashKey)
}

func fishSetup(binding string) string {
	// A binding with no fish 4 name gets the default rather than a broken bind
	// line: fish binds unrecognised text as a literal key sequence instead of
	// failing, which would silently steal those keystrokes.
	fishKey, ok := keybinding.ToFish(binding)
	if !ok {
		fishKey = "ctrl-t"
	}
	return fmt.Sprintf(`# termwise shell integration
set -gx TERMWISE_INVOKED_AS tw
function tw --description 'termwise'
    switch "$argv[1]"
        case ask config init help --help -h completion __complete __completeNoDesc
            command termwise $argv
            return
        case explain
            command termwise explain --session-id $fish_pid $argv[2..]
            return
    end
    if test (count $argv) -gt 0
        command termwise --session-id $fish_pid $argv
    else
        command termwise --session-id $fish_pid
    end
end

complete -c tw -w termwise

function _termwise_widget
    # string collect keeps a multi-line result as one value; $pipestatus[2] is
    # termwise's own exit code, since $status would be string collect's.
    set -l _tw_result (commandline | command termwise --shell-widget --session-id $fish_pid | string collect)
    set -l _tw_status $pipestatus[2]
    if test $_tw_status -ne 0
        commandline -f repaint
        return
    end
    if test -n "$_tw_result"
        commandline -r -- $_tw_result
    end
    commandline -f repaint
end

# Remove any previous _termwise_widget binding before applying the configured one.
for _tw_key in (bind --user | string replace -rf -- '^bind (\\S+) _termwise_widget$' '$1')
    bind -e -- $_tw_key
end
bind -- %s _termwise_widget
`, fishKey)
}
