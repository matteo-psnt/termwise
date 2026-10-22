package agentui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/theme"
)

func histSearchModel(t *testing.T, draft string, prompts ...string) Model {
	t.Helper()
	h := history.NewPromptHistory(t.TempDir())
	for _, p := range prompts {
		if err := h.Push(p); err != nil {
			t.Fatal(err)
		}
	}
	m := pickerModel(theme.DefaultName)
	m.promptHistory = h
	m.input.SetValue(draft)
	return m.enterHistSearch()
}

func TestHistSearchAcceptKeepsTheMatch(t *testing.T) {
	m := histSearchModel(t, "git", "ls -la", "git status")
	if v := m.input.Value(); v != "git status" {
		t.Fatalf("search shows %q, want the match %q", v, "git status")
	}

	m = pressKey(t, m, keyEnter)
	if m.state != stateIdle {
		t.Fatalf("enter left state %v, want idle", m.state)
	}
	if v := m.input.Value(); v != "git status" {
		t.Fatalf("enter left input %q, want the match", v)
	}
	if m.histSearch.query != "" || m.histSearch.matches != nil {
		t.Fatalf("enter left search state behind: %+v", m.histSearch)
	}
}

func TestHistSearchCancelRestoresTheDraft(t *testing.T) {
	m := histSearchModel(t, "git", "ls -la", "git status")

	m = pressKey(t, m, keyEsc)
	if m.state != stateIdle {
		t.Fatalf("esc left state %v, want idle", m.state)
	}
	if v := m.input.Value(); v != "git" {
		t.Fatalf("esc left input %q, want the draft %q", v, "git")
	}
	if m.histDraft != "" {
		t.Fatalf("esc kept histDraft %q", m.histDraft)
	}
	if m.histSearch.query != "" || m.histSearch.matches != nil {
		t.Fatalf("esc left search state behind: %+v", m.histSearch)
	}
}
