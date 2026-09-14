package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
)

var initCmd = &cobra.Command{
	Use:   "init <shell>",
	Short: "Output shell integration setup code",
	Long: `Output shell integration setup for your rc file.

Add to ~/.zshrc:
  eval "$(termwise init zsh)"

	Add to ~/.bashrc:
  eval "$(termwise init bash)"

  Add to ~/.config/fish/config.fish:
  termwise init fish | source`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(_ *cobra.Command, args []string) error {
		kb := readKeybinding()
		switch args[0] {
		case "zsh":
			fmt.Print(zshSetup(kb))
		case "bash":
			fmt.Print(bashSetup(kb))
		case "fish":
			fmt.Print(fishSetup(kb))
		default:
			return fmt.Errorf("unsupported shell %q — supported: zsh, bash, fish", args[0])
		}
		return nil
	},
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
            config|init|help|--help|-h)
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
tw() {
    case "$1" in
        ask|config|init|help|--help|-h)
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
function tw --description 'termwise'
    switch "$argv[1]"
        case ask config init help --help -h
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
