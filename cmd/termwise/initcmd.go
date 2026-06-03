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
  eval "$(termwise init bash)"`,
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
		default:
			return fmt.Errorf("unsupported shell %q — supported: zsh, bash", args[0])
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
    zle -I
    if [[ -n "$BUFFER" ]]; then
        command termwise --session-id "$$" --prefill "$BUFFER" 0<>/dev/tty >&0 2>&0
    else
        command termwise --session-id "$$" 0<>/dev/tty >&0 2>&0
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
    esac
    if [[ $# -gt 0 ]]; then
        command termwise --session-id "$$" "$@"
    else
        command termwise --session-id "$$"
    fi
}

_termwise_widget() {
    if [[ -n "$READLINE_LINE" ]]; then
        command termwise --session-id "$$" --prefill "$READLINE_LINE" 0<>/dev/tty >&0 2>&0
    else
        command termwise --session-id "$$" 0<>/dev/tty >&0 2>&0
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
