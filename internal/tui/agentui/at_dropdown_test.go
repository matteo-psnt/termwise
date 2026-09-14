package agentui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// atTestTree builds a fixed tree so the expectations below can be exact:
//
//	.hidden      cmd/      internal/      README.md  main.go
//	                       internal/agent/  internal/config.go
func atTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"cmd", "internal", filepath.Join("internal", "agent"), ".git"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"README.md", "main.go", ".hidden", filepath.Join("internal", "config.go")} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func labels(matches []slashMatch) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Label)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestComputeAtMatches(t *testing.T) {
	root := atTestTree(t)

	tests := []struct {
		name  string
		value string
		want  []string
	}{
		// Labels carry the whole fragment, not just the entry name, so the row
		// shows the path being built. Directories sort ahead of files and
		// dotfiles stay hidden.
		{"bare @ lists cwd", "explain @", []string{"cmd/", "internal/", "README.md", "main.go"}},
		{"prefix filters", "explain @in", []string{"internal/"}},
		{"descends into a directory", "explain @internal/", []string{"internal/agent/", "internal/config.go"}},
		{"prefix inside a directory", "explain @internal/con", []string{"internal/config.go"}},
		{"dotfiles need an explicit dot", "explain @.", []string{".git/", ".hidden"}},
		{"no @ token", "explain internal", nil},
		{"@ mid-word does not trigger", "mail foo@bar.com", nil},
		{"trailing space closes it", "explain @internal/ ", nil},
		{"nonexistent directory", "explain @nope/", nil},
		{"completes only the trailing token", "@cmd/ @in", []string{"internal/"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := labels(computeAtMatches(tc.value, root))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !equal(got, tc.want) {
				t.Errorf("computeAtMatches(%q) labels = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// The completion suffix is what makes descending a tree one keystroke: a
// directory reopens the dropdown, a file ends the token.
func TestComputeAtMatchesCompletion(t *testing.T) {
	root := atTestTree(t)

	dir := computeAtMatches("explain @intern", root)
	if len(dir) != 1 || dir[0].Completion != "al/" {
		t.Fatalf("directory completion = %+v, want suffix %q", dir, "al/")
	}
	file := computeAtMatches("explain @mai", root)
	if len(file) != 1 || file[0].Completion != "n.go " {
		t.Fatalf("file completion = %+v, want suffix %q", file, "n.go ")
	}
}

func TestAtFragment(t *testing.T) {
	tests := []struct {
		value string
		frag  string
		ok    bool
	}{
		{"@", "", true},
		{"look at @src/main", "src/main", true},
		{"@a\n@b", "b", true},
		{"foo@bar", "", false},
		{"no token here", "", false},
		{"@done ", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		frag, ok := atFragment(tc.value)
		if ok != tc.ok || frag != tc.frag {
			t.Errorf("atFragment(%q) = %q,%v; want %q,%v", tc.value, frag, ok, tc.frag, tc.ok)
		}
	}
}

// atModel builds an idle model whose input already holds value, with cwd
// pointed at the fixture tree. The contexts are the ones submitMessage needs,
// so a key that falls through to submit does not panic.
func atModel(t *testing.T, value, cwd string, suggestion string) Model {
	t.Helper()
	ta := textarea.New()
	ta.SetValue(value)
	ta.CursorEnd()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())
	t.Cleanup(suggestionCancel)

	return Model{
		shell: shell{
			input:            ta,
			cwd:              cwd,
			suggestion:       suggestion,
			histIdx:          -1,
			ctx:              ctx,
			cancel:           cancel,
			suggestionCtx:    suggestionCtx,
			suggestionCancel: suggestionCancel,
			contextWindow:    32_000,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateIdle,
	}
}

// With a path fragment typed there is usually a pending ghost suggestion too.
// Tab must complete the path the dropdown is showing, not accept the ghost.
func TestTabCompletesPathRatherThanSuggestion(t *testing.T) {
	m := atModel(t, "explain @intern", atTestTree(t), "explain why the tests are slow")

	got, _ := idleMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if v := got.(Model).input.Value(); v != "explain @internal/" {
		t.Fatalf("Tab gave %q, want the completed path %q", v, "explain @internal/")
	}
}

// Enter accepts the highlighted path and must not submit: the highlighted row
// is usually a directory, and sending "@internal/" to the model is never what
// the keystroke meant.
func TestEnterAcceptsPathWithoutSubmitting(t *testing.T) {
	m := atModel(t, "explain @intern", atTestTree(t), "")

	gotModel, _ := idleMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	got := gotModel.(Model)

	if v := got.input.Value(); v != "explain @internal/" {
		t.Fatalf("Enter gave %q, want the completed path", v)
	}
	if got.state != stateIdle {
		t.Fatalf("Enter left state %v, want stateIdle — it must not submit", got.state)
	}
	if len(got.messages) != 0 {
		t.Fatalf("Enter submitted %d message(s); want none", len(got.messages))
	}
}

// Once a file is completed the token ends with a space, which closes the
// dropdown so Enter submits normally again.
func TestEnterSubmitsOnceTheDropdownIsClosed(t *testing.T) {
	m := atModel(t, "explain @main.go ", atTestTree(t), "")
	if matches := m.visibleAtMatches(); len(matches) != 0 {
		t.Fatalf("dropdown still open after a completed file: %v", labels(matches))
	}
}

// Typing a filename in full leaves the dropdown open on a row with nothing
// left to complete. Enter there must send the message rather than append a
// space and demand a second Enter.
func TestEnterSubmitsWhenPathIsAlreadyComplete(t *testing.T) {
	m := atModel(t, "explain @main.go", atTestTree(t), "")

	gotModel, _ := idleMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	got := gotModel.(Model)

	if len(got.messages) != 1 {
		t.Fatalf("Enter produced %d message(s), want 1 — it should submit", len(got.messages))
	}
	if got.messages[0].Content != "explain @main.go" {
		t.Fatalf("submitted %q, want the untouched prompt", got.messages[0].Content)
	}
}

// Tab on that same row still completes, appending the trailing space that ends
// the token.
func TestTabStillCompletesAnAlreadyCompletePath(t *testing.T) {
	m := atModel(t, "explain @main.go", atTestTree(t), "")

	gotModel, _ := idleMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	got := gotModel.(Model)

	if v := got.input.Value(); v != "explain @main.go " {
		t.Fatalf("Tab gave %q, want a trailing space", v)
	}
	if len(got.messages) != 0 {
		t.Fatalf("Tab submitted %d message(s); want none", len(got.messages))
	}
}

// A slash line belongs to the slash dropdown, so @ completion stays out of it.
func TestSlashLineGetsNoPathCompletion(t *testing.T) {
	m := atModel(t, "/model @int", atTestTree(t), "")
	if matches := m.visibleAtMatches(); len(matches) != 0 {
		t.Fatalf("@ dropdown opened on a slash line: %v", labels(matches))
	}
}
