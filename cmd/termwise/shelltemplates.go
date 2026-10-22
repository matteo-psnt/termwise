package main

import (
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
)

// The scripts `termwise init <shell>` prints. Each defines the tw function
// and binds the keybinding to a widget that opens termwise on the current
// command line. They set cliname.EnvVar so messages can say `tw` rather than
// `termwise` once the integration is loaded.

func zshSetup(binding string) string {
	return fmt.Sprintf(`# termwise shell integration
export %[1]s=tw
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
bindkey %[2]s _termwise_widget
`, cliname.EnvVar, shQuote(binding))
}

func bashSetup(binding string) string {
	bashKey := keybinding.ToBash(binding)
	return fmt.Sprintf(`# termwise shell integration
export %[1]s=tw
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
bind -x %[2]s
`, cliname.EnvVar, shQuote(`"`+readlineKeyseq(bashKey)+`": _termwise_widget`))
}

func fishSetup(binding string) string {
	// A binding with no fish 4 name gets the default rather than a broken bind
	// line: fish binds unrecognised text as a literal key sequence instead of
	// failing, which would silently steal those keystrokes.
	fishKey, ok := keybinding.ToFish(binding)
	if !ok {
		fishKey, _ = keybinding.ToFish(config.DefaultKeybinding)
	}
	return fmt.Sprintf(`# termwise shell integration
set -gx %[1]s tw
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
bind -- %[2]s _termwise_widget
`, cliname.EnvVar, fishKey)
}

// shQuote single-quotes s for bash and zsh. The keybinding comes from the
// config file, and Alt+' is stored as \e' — pasted between bare single
// quotes it leaves one open, and the whole eval'd script then fails to
// parse, taking `tw` down with it.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// readlineKeyseq escapes key for use inside a double-quoted readline key
// sequence: a `"` (Alt+") would end it early, and a trailing lone backslash
// (Alt+\ is stored as \e\) would escape the closing quote.
func readlineKeyseq(key string) string {
	key = strings.ReplaceAll(key, `"`, `\"`)
	trailing := len(key) - len(strings.TrimRight(key, `\`))
	if trailing%2 == 1 {
		key += `\`
	}
	return key
}
