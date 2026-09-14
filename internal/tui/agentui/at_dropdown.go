package agentui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The @-dropdown completes filesystem paths in the prompt. It deliberately
// mirrors the slash dropdown rather than sharing its code: the two differ in
// trigger, in where they source rows from, and in what Enter means, and the
// shared part is small enough that a common abstraction would be all branches.
//
// Completion only fires on the trailing token of the input, which is what lets
// this reuse the slash dropdown's append-and-CursorEnd model instead of
// splicing at an arbitrary cursor offset.

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
	if m.atClosed || m.histIdx >= 0 {
		return nil
	}
	// The slash dropdown wins when both could match, so "/cmd @path" completes
	// the command first.
	if len(m.visibleSlashMatches()) > 0 {
		return nil
	}
	return computeAtMatches(m.input.Value(), m.cwd)
}

// visibleDropdown returns the rows and highlight index for whichever completion
// dropdown is open, so the layout and render sites do not each have to restate
// the precedence between them. cursor is meaningless when matches is empty.
func (m Model) visibleDropdown() (matches []slashMatch, cursor int) {
	if slash := m.visibleSlashMatches(); len(slash) > 0 {
		return slash, m.slashCursor
	}
	return m.visibleAtMatches(), m.atCursor
}

// handleAtDropdownKey handles navigation and acceptance while the @-dropdown is
// visible. Returns (newModel, handled); when handled is false the caller falls
// through to the normal idle-key handler.
//
// Unlike the slash dropdown, Enter accepts without submitting. Submitting on
// Enter there is right because the highlighted row is a complete command; here
// the highlighted row is usually a directory, and sending "@internal/" to the
// model is never what the keystroke meant.
func (m Model) handleAtDropdownKey(msg tea.KeyPressMsg, matches []slashMatch) (Model, bool) {
	if m.atCursor >= len(matches) {
		m.atCursor = len(matches) - 1
	}
	if m.atCursor < 0 {
		m.atCursor = 0
	}
	switch msg.Code {
	case tea.KeyUp:
		if m.atCursor > 0 {
			m.atCursor--
		}
		return m, true
	case tea.KeyDown:
		if m.atCursor < len(matches)-1 {
			m.atCursor++
		}
		return m, true
	case tea.KeyTab, tea.KeyEnter:
		return m.acceptAtCompletion(matches), true
	case tea.KeyEscape:
		m.atClosed = true
		return m, true
	}
	switch msg.String() {
	case "ctrl+p":
		if m.atCursor > 0 {
			m.atCursor--
		}
		return m, true
	case "ctrl+n":
		if m.atCursor < len(matches)-1 {
			m.atCursor++
		}
		return m, true
	case "right":
		// Only consume Right at end-of-line so cursor movement still works mid-text.
		if m.inputCursorAtEnd() {
			return m.acceptAtCompletion(matches), true
		}
	}
	return m, false
}

// acceptAtCompletion appends the highlighted match's completion suffix to the
// input and resets the dropdown cursor.
func (m Model) acceptAtCompletion(matches []slashMatch) Model {
	if m.atCursor < 0 || m.atCursor >= len(matches) {
		return m
	}
	completion := matches[m.atCursor].Completion
	if completion == "" {
		return m
	}
	m.input.SetValue(m.input.Value() + completion)
	m.input.CursorEnd()
	m.atCursor = 0
	return m
}
