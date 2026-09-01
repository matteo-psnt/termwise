package agentui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// keyAction binds one or more keys to a handler, plus the display metadata used
// to render the help overlay and the input-row hint line. Keys are matched
// against tea.KeyMsg.String() ("enter", "esc", "1", "y", "ctrl+r", "pgup", …),
// which collapses what used to be split between msg.Type and msg.String()
// switches into a single uniform match.
//
// label and desc are the single source of truth for both the help overlay and
// the compact hint row. An empty label marks a hidden binding (e.g. scrolling)
// that is handled but never shown.
type keyAction struct {
	keys  []string
	label string
	desc  string
	run   func(Model) (tea.Model, tea.Cmd)
}

// keymap is an ordered list of bindings owned by one mode. Order matters: the
// first action whose keys match a keypress wins, and help/hints render in list
// order.
type keymap []keyAction

// handle runs the first action whose keys match msg. The bool reports whether a
// binding matched, so callers can fall through to residual handling (typing,
// suggestions, etc.) when it didn't.
func (km keymap) handle(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	s := msg.String()
	for _, a := range km {
		for _, k := range a.keys {
			if k == s {
				newM, cmd := a.run(m)
				return newM, cmd, true
			}
		}
	}
	return m, nil, false
}

// help returns the visible bindings as help-overlay rows. Hidden bindings
// (empty label or desc) are skipped.
func (km keymap) help() []binding {
	var out []binding
	for _, a := range km {
		if a.label != "" && a.desc != "" {
			out = append(out, binding{key: a.label, desc: a.desc})
		}
	}
	return out
}

// hints renders the compact input-row hint, e.g. "  [↵] run command   [esc]
// skip". Returns "" when no binding is visible.
func (km keymap) hints(m Model) string {
	var parts []string
	for _, a := range km {
		if a.label != "" && a.desc != "" {
			parts = append(parts, "["+a.label+"] "+a.desc)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return m.renderer.styles.ActionHints.Render("  " + strings.Join(parts, "   "))
}

// scrollKeys are the viewport scroll bindings shared by idle, thinking, and
// command-proposal modes. They carry no label, so they are handled silently and
// never appear in help or hints. Modes splice them in with append(modeKeys,
// scrollKeys...).
var scrollKeys = keymap{
	{keys: []string{"pgup"}, run: Model.scrollPageUp},
	{keys: []string{"pgdown"}, run: Model.scrollPageDown},
	{keys: []string{"end"}, run: Model.scrollToBottom},
}

func (m Model) scrollPageUp() (tea.Model, tea.Cmd) {
	m.vp.PageUp()
	m.userScrolled = !m.vp.AtBottom()
	return m, nil
}

func (m Model) scrollPageDown() (tea.Model, tea.Cmd) {
	m.vp.PageDown()
	m.userScrolled = !m.vp.AtBottom()
	return m, nil
}

func (m Model) scrollToBottom() (tea.Model, tea.Cmd) {
	m.userScrolled = false
	m.vp.GotoBottom()
	return m, nil
}
