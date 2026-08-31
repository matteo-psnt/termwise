package configui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ctrl+c always quits without saving — intercepted before any sub-model sees it.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
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

	// Normal state message handling.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case editorConnMsg:
		m.connChecked = true
		m.connOK = msg.ok
		if msg.err != nil {
			m.connErr = msg.err.Error()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleNormalKey(msg)
	}

	return m, nil
}

// ---------------------------------------------------------------------------
// Sub-model update handlers
// ---------------------------------------------------------------------------

func (m editorModel) updateModelPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.modelPicker.Update(msg)
	mp := newModel.(modelPickerModel)
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
	newModel, cmd := m.authEditor.Update(msg)
	ae := newModel.(authEditorModel)
	m.authEditor = &ae
	if ae.done {
		m.authEditor = nil
		if !ae.cancelled {
			existing := m.cfg.Providers[ae.provider]
			ae.result.Model = existing.Model
			config.SetProvider(&m.cfg, ae.provider, ae.result)
			m.resetConnectivity()
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}
	}
	return m, cmd
}

func (m editorModel) updateKeybinding(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.keybinding.Update(msg)
	kb := newModel.(keybindingModel)
	m.keybinding = &kb
	if kb.done {
		m.keybinding = nil
		if !kb.cancelled {
			config.Settings[m.activeSettingIdx].Set(&m.cfg, kb.result)
		}
	}
	return m, cmd
}

func (m editorModel) updateThemePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevPreview := m.themePicker.PreviewName()
	newModel, cmd := m.themePicker.Update(msg)
	tp := newModel.(themePickerModel)
	m.themePicker = &tp
	if tp.done {
		m.themePicker = nil
		if !tp.cancelled {
			config.Settings[m.activeSettingIdx].Set(&m.cfg, tp.result)
			m.styles = newStylesForTheme(m.r, tp.result)
		} else {
			m.styles = newStylesForTheme(m.r, tp.prev)
		}
	} else if tp.PreviewName() != prevPreview {
		m.styles = newStylesForTheme(m.r, tp.PreviewName())
	}
	return m, cmd
}

func (m editorModel) updateEnumPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.enumPicker.Update(msg)
	ep := newModel.(enumPickerModel)
	m.enumPicker = &ep
	if !ep.done {
		return m, cmd
	}
	m.enumPicker = nil
	if ep.cancelled {
		m.activeEffortProvider = ""
		return m, cmd
	}
	if m.activeEffortProvider != "" {
		pc := m.cfg.Providers[m.activeEffortProvider]
		// Omit when the picker chose the default — keeps TOML clean.
		if ep.result == config.DefaultEffort {
			pc.Effort = ""
		} else {
			pc.Effort = ep.result
		}
		config.SetProvider(&m.cfg, m.activeEffortProvider, pc)
		m.activeEffortProvider = ""
	} else {
		config.Settings[m.activeSettingIdx].Set(&m.cfg, ep.result)
	}
	return m, cmd
}

func (m editorModel) updateAddWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.addWizard.Update(msg)
	newWiz := newModel.(wizardModel)
	m.addWizard = &newWiz
	if newWiz.Result != nil {
		for name, pc := range newWiz.Result.Providers {
			config.SetProvider(&m.cfg, name, pc)
		}
		m.addWizard = nil
		m.buildRows()
		m.resetConnectivity()
		return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
	}
	if newWiz.done {
		m.addWizard = nil
		return m, nil
	}
	return m, cmd
}

// ---------------------------------------------------------------------------
// Normal state key handling
// ---------------------------------------------------------------------------

func (m editorModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	m.cfg = m.initial.Clone()
	m.styles = newStylesForTheme(m.r, m.cfg.Settings.Theme)
	m.buildRows()
	m.resetConnectivity()
	return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
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
		mp := newModelPicker(row.provider, pc, pc.Model, m.styles)
		m.modelPicker = &mp
		return m, m.modelPicker.Init()

	case rowProvEffort:
		current := m.cfg.Providers[row.provider].Effort
		if current == "" {
			current = config.DefaultEffort
		}
		ep := newEnumPicker("Reasoning effort", current, config.EffortLevels, m.styles)
		m.enumPicker = &ep
		m.activeEffortProvider = row.provider
		return m, nil

	case rowProvAuth:
		ae := newAuthEditor(
			row.provider,
			m.cfg.Providers[row.provider],
			row.provider == m.cfg.SelectedProvider,
			m.connOK,
			m.styles,
		)
		m.authEditor = &ae
		return m, m.authEditor.Init()

	case rowAddProvider:
		exclude := make(map[string]bool, len(m.cfg.Providers))
		for name := range m.cfg.Providers {
			exclude[name] = true
		}
		wiz := newWizardModel(m.r, theme.Normalize(m.cfg.Settings.Theme))
		wiz.exclude = exclude
		m.addWizard = &wiz
		return m, m.addWizard.Init()

	case rowSetting:
		return activateSetting(m, row.settingIdx), nil
	}
	return m, nil
}

func (m editorModel) saveAndQuit() (tea.Model, tea.Cmd) {
	if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
		m.err = err
	}
	return m, tea.Quit
}
