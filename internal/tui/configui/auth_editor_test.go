package configui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
)

func pressAuth(m authEditorModel, keys ...tea.KeyPressMsg) authEditorModel {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func TestAuthEditorPrefillsExistingValue(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	pc := config.ProviderConfig{Auth: "keychain", APIKeyCmd: "pass show openai"}
	m := newAuthEditor("openai", pc, true, true, newStylesForTheme(true, ""))

	m = pressAuth(m, keyDown, keyDown, keyEnter) // env, keychain, cmd
	if m.step != authStepEnterValue {
		t.Fatalf("step = %v, want value entry", m.step)
	}
	if got := m.input.Value(); got != "pass show openai" {
		t.Errorf("input = %q, want the existing command", got)
	}
	view := m.View()
	if !strings.Contains(view, "Shell command:") || !strings.Contains(view, "enter verify & save") {
		t.Errorf("unexpected view:\n%s", view)
	}
}

func TestAuthEditorRequiresAPIKey(t *testing.T) {
	m := newAuthEditor("openai", config.ProviderConfig{}, true, true, newStylesForTheme(true, ""))
	m = pressAuth(m, keyDown, keyEnter) // keychain
	if m.input.EchoMode != textinput.EchoPassword {
		t.Error("API key is not masked")
	}
	m = pressAuth(m, keyEnter)
	if m.step != authStepEnterValue || m.inputErr != "value cannot be empty" {
		t.Fatalf("step = %v, inputErr = %q; want to stay with an error", m.step, m.inputErr)
	}
	if !strings.Contains(m.View(), "Error: value cannot be empty") {
		t.Errorf("error not shown:\n%s", m.View())
	}
}

func TestWizardBlankFallbackUsesDefault(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	m := newWizardModel(true, "", map[string]bool{})
	m.provider = "openai"
	m.authMethod = "env"
	m.setupEnterValue()
	m.step = wizEnterValue

	next, cmd := m.handleEnterValueKey(keyEnter)
	if got := next.(wizardModel).step; got != wizWorking || cmd == nil {
		t.Errorf("step = %v; want the blank env var to fall back and connect", got)
	}

	m.authMethod = "cmd"
	m.setupEnterValue()
	next, _ = m.handleEnterValueKey(keyEnter)
	if got := next.(wizardModel).step; got != wizEnterValue {
		t.Errorf("step = %v; want a blank command to be refused", got)
	}
}

func TestAuthEditKeepsRestOfProviderBlock(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cfg := config.FileConfig{
		SelectedProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {Auth: "env", Model: "gpt-5", Effort: "high", BaseURL: "https://proxy.example/v1"},
		},
	}
	m := newEditorModel("", cfg, true)
	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowProvAuth })
	m = press(t, m, keyDown, keyDown, keyEnter) // cmd
	m.authEditor.input.SetValue("pass show openai")
	m = press(t, m, keyEnter)
	m, _ = update(t, m, editorAuthMsg{})
	if m.authEditor != nil {
		t.Fatal("auth editor still open after a successful verify")
	}
	want := config.ProviderConfig{
		Auth: "cmd", APIKeyCmd: "pass show openai",
		Model: "gpt-5", Effort: "high", BaseURL: "https://proxy.example/v1",
	}
	if got := m.cfg.Providers["openai"]; got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
