package configui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/config"
)

// fakeKeychain swaps the keychain operations for an in-memory map, seeded
// with entries, for the rest of the test.
func fakeKeychain(t *testing.T, entries map[string]string) map[string]string {
	t.Helper()
	get, set, del := keychainGet, keychainSet, keychainDelete
	t.Cleanup(func() { keychainGet, keychainSet, keychainDelete = get, set, del })
	keychainGet = func(entry string) (string, error) {
		v, ok := entries[entry]
		if !ok {
			return "", keyring.ErrNotFound
		}
		return v, nil
	}
	keychainSet = func(entry, key string) error { entries[entry] = key; return nil }
	keychainDelete = func(entry string) error {
		if _, ok := entries[entry]; !ok {
			return keyring.ErrNotFound
		}
		delete(entries, entry)
		return nil
	}
	return entries
}

// replaceOpenAIKey opens openai's auth editor on its current keychain
// method, enters sk-new and verifies it against keychainEditor's server.
func replaceOpenAIKey(t *testing.T, m editorModel) editorModel {
	t.Helper()
	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowProvAuth })
	m = press(t, m, keyEnter)
	m.authEditor.input.SetValue("sk-new")
	m = press(t, m, keyEnter)
	if m.authEditor == nil || m.authEditor.step != authStepVerify {
		t.Fatal("key not sent for verification")
	}
	m, _ = update(t, m, m.authEditor.verifyCmd()())
	if m.authEditor != nil {
		t.Fatalf("verification failed: %s", m.authEditor.inputErr)
	}
	return m
}

func keychainEditor(t *testing.T) editorModel {
	t.Helper()
	srv := fakeOpenAI(t, "sk-new")
	cfg := config.FileConfig{
		SelectedProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai"), BaseURL: srv.URL},
		},
	}
	return newEditorModel("", cfg, true)
}

func TestEditorRevertRestoresKeychain(t *testing.T) {
	entry := config.DefaultKeychainEntry("openai")
	store := fakeKeychain(t, map[string]string{entry: "sk-old"})

	m := replaceOpenAIKey(t, keychainEditor(t))
	if store[entry] != "sk-new" {
		t.Fatalf("keychain = %q, want the verified key written", store[entry])
	}
	// A second key in the same session must still revert to the first.
	m = replaceOpenAIKey(t, m)
	m = press(t, m, keyEsc)
	if store[entry] != "sk-old" {
		t.Errorf("keychain = %q after revert, want the old key back", store[entry])
	}
	if m.err != nil {
		t.Errorf("err = %v", m.err)
	}

	// Saved sessions keep the new key: only a revert undoes it.
	m = replaceOpenAIKey(t, m)
	m.cfgPath = t.TempDir() + "/config.toml"
	next, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if store[entry] != "sk-new" {
		t.Errorf("keychain = %q after save, want the new key kept", store[entry])
	}
	if err := next.(editorModel).err; err != nil {
		t.Errorf("save: %v", err)
	}
}

func TestEditorRevertDeletesNewKeychainEntry(t *testing.T) {
	store := fakeKeychain(t, map[string]string{})
	m := replaceOpenAIKey(t, keychainEditor(t))
	m = press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}) // quit without saving
	if v, ok := store[config.DefaultKeychainEntry("openai")]; ok {
		t.Errorf("keychain holds %q after quitting unsaved, want the entry gone", v)
	}
	if m.err != nil {
		t.Errorf("err = %v", m.err)
	}
}

// A key the add-provider wizard verified and stored belongs to a provider
// that is never added if the wizard is abandoned, so its entry is put back.
func TestAbandonedWizardRestoresKeychain(t *testing.T) {
	entry := config.DefaultKeychainEntry("anthropic")
	store := fakeKeychain(t, map[string]string{entry: "sk-old"})

	m := newEditorModel("", config.FileConfig{}, true)
	wiz := newWizardModel(true, "", nil)
	wiz.keychain = newKeychainJournal()
	if err := wiz.keychain.store("anthropic", "sk-new"); err != nil {
		t.Fatal(err)
	}
	m.addWizard = &wiz

	next, _ := m.updateAddWizard(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if next.(editorModel).addWizard != nil {
		t.Fatal("q did not close the wizard")
	}
	if store[entry] != "sk-old" {
		t.Errorf("keychain = %q after abandoning the wizard, want sk-old", store[entry])
	}
}

// Finishing the wizard hands its writes to the session, so reverting the
// session still undoes them.
func TestAdoptKeepsTheSessionsOlderValue(t *testing.T) {
	entry := config.DefaultKeychainEntry("openai")
	store := fakeKeychain(t, map[string]string{entry: "sk-1"})
	session, child := newKeychainJournal(), newKeychainJournal()
	if err := session.store("openai", "sk-2"); err != nil {
		t.Fatal(err)
	}
	if err := child.store("openai", "sk-3"); err != nil {
		t.Fatal(err)
	}
	session.adopt(child)
	if err := session.restore(); err != nil {
		t.Fatal(err)
	}
	if store[entry] != "sk-1" {
		t.Errorf("keychain = %q after revert, want the value from before the session", store[entry])
	}
}
