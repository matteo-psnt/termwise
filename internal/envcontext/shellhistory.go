package envcontext

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// recentCommandCount is how much shell history is worth sending. "Fix that last
// command" needs the last few lines; more than this is noise the model has to
// pay for on every request.
const recentCommandCount = 15

// secretish matches history lines likely to contain a credential typed inline.
// Shell history is the classic place secrets leak from, so anything that looks
// like one is dropped entirely rather than redacted — a partially-redacted line
// is rarely useful context anyway.
var secretish = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|credential|bearer|auth[_-]?header|-----BEGIN)`)

// selfInvocation drops termwise's own commands, which are never useful context
// and would otherwise dominate the recent history of an active user.
var selfInvocation = regexp.MustCompile(`^\s*(tw|termwise)\b`)

// zshExtended matches zsh's EXTENDED_HISTORY line format:
//
//	: 1774724113:0;git status
var zshExtended = regexp.MustCompile(`^:\s*\d+:\d+;(.*)$`)

// RecentCommands returns the most recent shell commands, oldest first, with
// probable secrets and termwise's own invocations removed. It returns nil when
// no history file is readable — this is best-effort context, never a hard
// dependency.
func RecentCommands(shell string) []string {
	path := historyPath(shell)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only

	zsh := strings.Contains(shell, "zsh")
	var kept []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		raw := sc.Bytes()
		if zsh {
			raw = unmetafy(raw)
		}
		line := string(raw)
		if m := zshExtended.FindStringSubmatch(line); m != nil {
			line = m[1]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if secretish.MatchString(line) || selfInvocation.MatchString(line) {
			continue
		}
		// Collapse an immediately repeated command.
		if len(kept) > 0 && kept[len(kept)-1] == line {
			continue
		}
		kept = append(kept, line)
		// Keep only a trailing window so a huge history file costs no more memory
		// than a small one.
		if len(kept) > recentCommandCount*4 {
			kept = kept[len(kept)-recentCommandCount:]
		}
	}
	if len(kept) > recentCommandCount {
		kept = kept[len(kept)-recentCommandCount:]
	}
	return kept
}

// zshMeta is the byte zsh writes ahead of each "metafied" byte in its
// history file; the byte that follows is the original XOR 0x20.
const zshMeta = 0x83

// unmetafy reverses zsh's history encoding. Without it every non-ASCII
// command (accented paths, CJK file names) comes out garbled, since UTF-8
// multibyte sequences are exactly the bytes zsh metafies. Only zsh history is
// encoded this way: in bash history 0x83 is just a UTF-8 continuation byte.
func unmetafy(b []byte) []byte {
	if bytes.IndexByte(b, zshMeta) < 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == zshMeta && i+1 < len(b) {
			i++
			out = append(out, b[i]^0x20)
			continue
		}
		out = append(out, b[i])
	}
	return out
}

// historyPath resolves the shell's history file: $HISTFILE when the shell
// exports it, otherwise the conventional location for the detected shell.
func historyPath(shell string) string {
	if p := os.Getenv("HISTFILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zsh_history")
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bash_history")
	default:
		return ""
	}
}
