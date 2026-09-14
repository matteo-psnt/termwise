package agentui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The @-dropdown completes filesystem paths in the prompt. Highlight movement,
// dismissal and acceptance are shared with the slash dropdown in completion.go;
// what lives here is the trigger, the listing, and Enter.
//
// Completion only fires on the trailing token of the input, which is what lets
// this reuse the append-and-CursorEnd model instead of splicing at an arbitrary
// cursor offset.

// atFragment returns the path fragment being completed — the text after the
// leading "@" of the input's trailing token — and whether the input is in a
// state where the dropdown should be open at all.
//
// The token must begin the word, so "foo@bar.com" does not trigger. Splitting
// on the last whitespace is what enforces that: the resulting token only starts
// with "@" when the "@" was at a word boundary.
func atFragment(value string) (string, bool) {
	if strings.HasSuffix(value, " ") || strings.HasSuffix(value, "\n") {
		return "", false
	}
	token := value
	if i := strings.LastIndexAny(value, " \t\n"); i >= 0 {
		token = value[i+1:]
	}
	if !strings.HasPrefix(token, "@") {
		return "", false
	}
	return token[1:], true
}

// computeAtMatches lists the completions for the @-token at the end of value.
// dir entries sort before files so that descending a tree stays a single
// keystroke, and dotfiles stay hidden unless the fragment is reaching for one —
// the same rule the shell uses, and the reason .git does not crowd the list.
func computeAtMatches(value, cwd string) []slashMatch {
	frag, ok := atFragment(value)
	if !ok {
		return nil
	}

	// Split the fragment into the directory to list and the prefix to match.
	dirPart, base := "", frag
	if i := strings.LastIndex(frag, "/"); i >= 0 {
		dirPart, base = frag[:i+1], frag[i+1:]
	}

	listDir := dirPart
	switch {
	case strings.HasPrefix(dirPart, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		listDir = filepath.Join(home, dirPart[2:])
	case filepath.IsAbs(dirPart):
		// already absolute
	default:
		listDir = filepath.Join(cwd, dirPart)
	}

	entries, err := os.ReadDir(listDir)
	if err != nil {
		return nil
	}

	showHidden := strings.HasPrefix(base, ".")
	type row struct {
		name  string
		isDir bool
	}
	var rows []row
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(name, base) {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&os.ModeSymlink != 0 {
			// A symlink to a directory should complete like a directory.
			if info, err := os.Stat(filepath.Join(listDir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		rows = append(rows, row{name: name, isDir: isDir})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].isDir != rows[j].isDir {
			return rows[i].isDir
		}
		return rows[i].name < rows[j].name
	})

	matches := make([]slashMatch, 0, len(rows))
	for _, r := range rows {
		// A directory completes to a trailing "/" so the dropdown immediately
		// reopens on its contents; a file completes to a trailing space, which
		// ends the token and closes the dropdown.
		suffix := " "
		label := dirPart + r.name
		if r.isDir {
			suffix = "/"
			label += "/"
		}
		matches = append(matches, slashMatch{
			Label:      label,
			Completion: r.name[len(base):] + suffix,
		})
	}
	return matches
}

// visibleAtMatches returns the @-dropdown rows to render now, or nil when the
// dropdown should be hidden.
func (m Model) visibleAtMatches() []slashMatch {
	if m.atSel.closed || m.histIdx >= 0 {
		return nil
	}
	// A slash line belongs to the slash dropdown. Testing the prefix rather than
	// calling visibleSlashMatches keeps this off the LoadConfig path that
	// computeSlashMatches walks for arg completion — this runs per keystroke,
	// from several call sites per frame.
	if strings.HasPrefix(m.input.Value(), "/") {
		return nil
	}
	return computeAtMatches(m.input.Value(), m.cwd)
}

// handleAtDropdownKey handles navigation and acceptance while the @-dropdown is
// visible. Returns (newModel, handled); when handled is false the caller falls
// through to the normal idle-key handler.
//
// Only Enter differs from the slash dropdown. There, Enter accepts and submits,
// because the highlighted row is a complete command. Here the highlighted row
// is usually a directory, and sending "@internal/" to the model is never what
// the keystroke meant — so Enter accepts and stops, unless there is nothing
// left to complete.
func (m Model) handleAtDropdownKey(msg tea.KeyPressMsg, matches []slashMatch) (Model, bool) {
	if msg.Code == tea.KeyEnter {
		m.atSel.clamp(len(matches))
		// A lone space means the highlighted row is a file the user has already
		// typed in full. Submit it rather than making them press Enter twice.
		if matches[m.atSel.cursor].Completion == " " {
			return m, false
		}
		return m.acceptCompletion(matches, &m.atSel), true
	}
	return m.handleCompletionNav(msg, matches, &m.atSel)
}
