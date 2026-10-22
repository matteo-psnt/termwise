package configui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The editor's own messages are handled before delegating: a sub-model
	// would drop them, freezing the connectivity spinner and leaving the
	// active provider's indicator stale.
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// ctrl+c always quits without saving — intercepted before any sub-model sees it.
		// Keys written this session are discarded along with the rest.
		if msg.String() == "ctrl+c" {
			m.restoreKeychain()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		// Kept here and also passed on below, so whichever of the editor and
		// the open sub-model draws next has the current size.
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		// Sub-models run spinners of their own; the ID says whose tick it is.
		if msg.ID == m.spin.ID() {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}

	case editorConnMsg:
		if msg.provider != m.cfg.SelectedProvider || msg.seq != m.connSeq {
			return m, nil
		}
		m.connChecked = true
		m.connOK = msg.err == nil
		return m, nil
	}

	// Delegate to the active sub-model.
	switch {
	case m.modelPicker != nil:
		return m.updateModelPicker(msg)
	case m.authEditor != nil:
		return m.updateAuthEditor(msg)
	case m.keybinding != nil:
		return m.updateKeybinding(msg)
	case m.themePicker != nil:
		return m.updateThemePicker(msg)
	case m.enumPicker != nil:
		return m.updateEnumPicker(msg)
	case m.addWizard != nil:
		return m.updateAddWizard(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleNormalKey(key)
	}

	return m, nil
}

// ---------------------------------------------------------------------------
// Sub-model update handlers
// ---------------------------------------------------------------------------

func (m editorModel) updateModelPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	mp, cmd := m.modelPicker.Update(msg)
	m.modelPicker = &mp
	if mp.done {
		m.modelPicker = nil
		if !mp.cancelled && mp.selected != "" {
			pc := m.cfg.Providers[mp.provider]
			pc.Model = mp.selected
			config.SetProvider(&m.cfg, mp.provider, pc)
			// Switching model may toggle thinking-support, which adds or removes
			// the per-provider Effort row.
			m.buildRows()
		}
	}
	return m, cmd
}

func (m editorModel) updateAuthEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	ae, cmd := m.authEditor.Update(msg)
	m.authEditor = &ae
	if ae.done {
		m.authEditor = nil
		if !ae.cancelled {
			config.SetProvider(&m.cfg, ae.provider, ae.result)
			m.resetConnectivity()
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}
	}
	return m, cmd
}

func (m editorModel) updateKeybinding(msg tea.Msg) (tea.Model, tea.Cmd) {
	kb, cmd := m.keybinding.Update(msg)
	m.keybinding = &kb
	if kb.done {
		m.keybinding = nil
		if !kb.cancelled {
			m.commit(&m.cfg, kb.result)
		}
	}
	return m, cmd
}

func (m editorModel) updateThemePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevPreview := m.themePicker.PreviewName()
	tp, cmd := m.themePicker.Update(msg)
	m.themePicker = &tp
	if tp.done {
		m.themePicker = nil
		if !tp.cancelled {
			m.commit(&m.cfg, tp.result)
			m.styles = newStylesForTheme(m.hasDarkBg, tp.result)
		} else {
			m.styles = newStylesForTheme(m.hasDarkBg, tp.prev)
		}
	} else if tp.PreviewName() != prevPreview {
		m.styles = newStylesForTheme(m.hasDarkBg, tp.PreviewName())
	}
	return m, cmd
}

func (m editorModel) updateEnumPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	ep, cmd := m.enumPicker.Update(msg)
	m.enumPicker = &ep
	if !ep.done {
		return m, cmd
	}
	m.enumPicker = nil
	if !ep.cancelled {
		m.commit(&m.cfg, ep.result)
	}
	return m, cmd
}

func (m editorModel) updateAddWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.addWizard.Update(msg)
	newWiz := newModel.(wizardModel)
	m.addWizard = &newWiz
	if newWiz.result != nil {
		m.keychain.adopt(newWiz.keychain)
		for name, pc := range newWiz.result.Providers {
			config.SetProvider(&m.cfg, name, pc)
		}
		m.addWizard = nil
		m.buildRows()
		m.resetConnectivity()
		return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
	}
	if newWiz.done {
		// Abandoned: a key it verified and stored belongs to a provider that
		// was never added, so put the entry back as it was.
		if err := newWiz.keychain.restore(); err != nil {
			m.err = fmt.Errorf("restoring keychain: %w", err)
		}
		m.addWizard = nil
		return m, nil
	}
	return m, cmd
}

// ---------------------------------------------------------------------------
// Normal state key handling
// ---------------------------------------------------------------------------

func (m editorModel) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.saveAndQuit()
	case "esc":
		return m.revertToSnapshot()
	case "up", "k":
		return m.moveCursor(-1), nil
	case "down", "j":
		return m.moveCursor(1), nil
	case "enter", " ":
		return m.activateRow()
	}
	return m, nil
}

// revertToSnapshot rolls in-memory edits back to the config as it was when the
// editor opened. The editor stays open; nothing is written to disk.
func (m editorModel) revertToSnapshot() (tea.Model, tea.Cmd) {
	m.restoreKeychain()
	m.cfg = m.initial.Clone()
	m.styles = newStylesForTheme(m.hasDarkBg, m.cfg.Settings.Theme)
	m.buildRows()
	m.resetConnectivity()
	return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
}

// restoreKeychain puts back the keychain entries written since the snapshot.
// It runs inline rather than as a Cmd so a key written right after a revert
// can't race it; no write is ever in flight here, since the auth editor and
// wizard take no keys while verifying. A failure is reported on exit, as the
// editor has nowhere else to show it.
func (m *editorModel) restoreKeychain() {
	if err := m.keychain.restore(); err != nil {
		m.err = fmt.Errorf("restoring keychain: %w", err)
	}
}

func (m editorModel) activateRow() (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	row := m.rows[m.cursor]
	switch row.kind {
	case rowProvHeader:
		if row.provider != m.cfg.SelectedProvider {
			m.cfg.SelectedProvider = row.provider
			m.resetConnectivity()
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}

	case rowProvModel:
		pc := m.cfg.Providers[row.provider]
		m.pickerSeq++
		mp := newModelPicker(row.provider, pc, pc.Model, m.pickerSeq, m.height, m.styles)
		m.modelPicker = &mp
		return m, m.modelPicker.Init()

	case rowProvEffort:
		current := m.cfg.Providers[row.provider].Effort
		if current == "" {
			current = config.DefaultEffort
		}
		ep := newEnumPicker("Reasoning effort", current, config.EffortLevels, m.styles)
		m.enumPicker = &ep
		providerName := row.provider
		m.commit = func(cfg *config.FileConfig, val string) {
			pc := cfg.Providers[providerName]
			// Omit when the picker chose the default — keeps TOML clean.
			if val == config.DefaultEffort {
				pc.Effort = ""
			} else {
				pc.Effort = val
			}
			config.SetProvider(cfg, providerName, pc)
		}
		return m, nil

	case rowProvAuth:
		ae := newAuthEditor(row.provider, m.cfg.Providers[row.provider], m.keychain, m.styles)
		m.authEditor = &ae
		return m, m.authEditor.Init()

	case rowAddProvider:
		exclude := make(map[string]bool, len(m.cfg.Providers))
		for name := range m.cfg.Providers {
			exclude[name] = true
		}
		wiz := newWizardModel(m.hasDarkBg, theme.Normalize(m.cfg.Settings.Theme), exclude)
		// The window size arrived before the wizard existed; it sizes its
		// model list from it.
		wiz.width, wiz.height = m.width, m.height
		// The wizard keeps its own journal: abandoning it undoes just its
		// writes, and finishing hands them to the session's journal.
		wiz.keychain = newKeychainJournal()
		m.addWizard = &wiz
		return m, m.addWizard.Init()

	case rowSetting:
		return m.activateSetting(row.settingIdx), nil
	}
	return m, nil
}

// activateSetting handles Enter on a setting row: toggles booleans inline,
// opens the appropriate sub-model for all other kinds.
func (m editorModel) activateSetting(settingIdx int) editorModel {
	def := config.Settings[settingIdx]
	switch def.Kind {
	case config.KindToggle:
		current, _ := def.GetAny(m.cfg).(bool)
		if current {
			def.Set(&m.cfg, "Off")
		} else {
			def.Set(&m.cfg, "On")
		}
	case config.KindTheme:
		tp := newThemePicker(def.Get(m.cfg))
		m.themePicker = &tp
		m.commit = def.Set
	case config.KindKeybinding:
		kb := def.Resolve(m.cfg)
		kbm := newKeybindingCapture(kb, m.styles)
		m.keybinding = &kbm
		m.commit = def.Set
	case config.KindEnum:
		ep := newEnumPicker(def.Label, def.Resolve(m.cfg), def.Options, m.styles)
		m.enumPicker = &ep
		m.commit = def.Set
	}
	return m
}

func (m editorModel) saveAndQuit() (tea.Model, tea.Cmd) {
	if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
		m.err = err
	}
	return m, tea.Quit
}
