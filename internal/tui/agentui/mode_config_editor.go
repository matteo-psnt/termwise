package agentui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
)

// configEditorMode is active while the /config two-level settings editor is
// open. The chooser picks a setting; the per-setting value picker (or
// keybinding capture) then commits the change.
type configEditorMode struct{}

func (configEditorMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.configEditor == nil {
		m.state = stateIdle
		return m, nil
	}
	switch m.configEditor.phase {
	case configPhaseChooser:
		return handleConfigChooserKey(m, msg)
	case configPhaseValue:
		return handleConfigValueKey(m, msg)
	case configPhaseCapture:
		return handleConfigCaptureKey(m, msg)
	}
	return m, nil
}

func handleConfigChooserKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	updated, choice, submitted, cancelled := m.configEditor.chooser.Update(msg)
	m.configEditor.chooser = updated
	if cancelled {
		m.configEditor = nil
		m.state = stateIdle
		return m, nil
	}
	if submitted {
		def, ok := config.FindSetting(choice)
		if !ok {
			return m, nil
		}
		return openConfigValuePicker(m, def), nil
	}
	return m, nil
}

// openConfigValuePicker transitions from the chooser to a per-setting value
// editor. KindKeybinding switches into capture mode; everything else opens a
// slashPicker shaped by the setting's Kind.
func openConfigValuePicker(m Model, def config.SettingDef) Model {
	if def.Kind == config.KindKeybinding {
		m.configEditor.setting = def
		m.configEditor.original = def.Resolve(m.configEditor.cfg)
		m.configEditor.valuePicker = nil
		m.configEditor.phase = configPhaseCapture
		return m
	}
	opts := valuePickerOptions(def)
	if opts == nil {
		return m
	}
	current := def.Resolve(m.configEditor.cfg)
	p := newSlashPicker("/config", def.Label, def.Description, opts, current)
	m.configEditor.setting = def
	m.configEditor.valuePicker = &p
	m.configEditor.original = current
	m.configEditor.phase = configPhaseValue
	return m
}

// handleConfigCaptureKey reads a single keypress as the new keybinding value.
// Esc cancels (no change). Any other key with a canonical zsh translation is
// captured, applied in memory, and persisted. Keys with no translation (e.g.
// raw modifier-only events) are ignored so the user can try again.
func handleConfigCaptureKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		m = backToChooser(m)
		m.refreshViewport()
		return m, nil
	}
	zsh, ok := keybinding.KeyMsgToZsh(msg.Key())
	if !ok {
		return m, nil
	}
	def := m.configEditor.setting
	m = applyConfigValue(m, def, zsh)
	m.persistConfigValue(def, zsh)
	def.Set(&m.configEditor.cfg, zsh)
	m.configEditor.chooser = rebuildChooser(m.configEditor.chooser, m.configEditor.cfg)
	m = backToChooser(m)
	m.refreshViewport()
	return m, nil
}

func handleConfigValueKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.configEditor.valuePicker == nil {
		m.configEditor.phase = configPhaseChooser
		return m, nil
	}
	prevCursor := m.configEditor.valuePicker.cursor
	updated, choice, submitted, cancelled := m.configEditor.valuePicker.Update(msg)
	m.configEditor.valuePicker = &updated
	def := m.configEditor.setting
	if cancelled {
		m = applyConfigValue(m, def, m.configEditor.original)
		m = backToChooser(m)
		m.refreshViewport()
		return m, nil
	}
	if submitted {
		m = applyConfigValue(m, def, choice)
		m.persistConfigValue(def, choice)
		def.Set(&m.configEditor.cfg, choice)
		m.configEditor.chooser = rebuildChooser(m.configEditor.chooser, m.configEditor.cfg)
		m = backToChooser(m)
		m.refreshViewport()
		return m, nil
	}
	if updated.cursor != prevCursor && len(updated.options) > 0 {
		m = applyConfigValue(m, def, updated.options[updated.cursor].Value)
		m.refreshViewport()
	}
	return m, nil
}

// backToChooser clears value-phase state and returns to the chooser view.
func backToChooser(m Model) Model {
	m.configEditor.phase = configPhaseChooser
	m.configEditor.valuePicker = nil
	m.configEditor.setting = config.SettingDef{}
	m.configEditor.original = ""
	return m
}

// rebuildChooser returns a chooser with refreshed Detail values from the
// updated config snapshot, preserving the current cursor position.
func rebuildChooser(old slashPicker, cfg config.FileConfig) slashPicker {
	next := newSlashPicker(old.cmd, old.title, old.subtitle, configChooserOptions(cfg), "")
	if old.cursor < len(next.options) {
		next.cursor = old.cursor
	}
	return next
}

func (configEditorMode) renderInputRow(m Model, _ int) string {
	if m.configEditor == nil {
		return ""
	}
	switch m.configEditor.phase {
	case configPhaseChooser:
		return m.configEditor.chooser.View(m.renderer)
	case configPhaseValue:
		if m.configEditor.valuePicker != nil {
			return m.configEditor.valuePicker.View(m.renderer)
		}
	case configPhaseCapture:
		return renderCaptureView(m)
	}
	return ""
}

// renderCaptureView shows the inline "Press a key…" prompt used when the user
// is rebinding the shell hotkey. Matches the visual rhythm of the value picker:
// title + subtitle + body + hint line.
func renderCaptureView(m Model) string {
	def := m.configEditor.setting
	var b strings.Builder
	b.WriteString(m.renderer.styles.InputPrompt.Render(" "+def.Label) + "\n")
	if def.Description != "" {
		b.WriteString(m.renderer.styles.ActionHints.Render("  "+def.Description) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.renderer.styles.InputPrompt.Render("  Press a key…") + "\n")
	b.WriteString("\n")
	b.WriteString(m.renderer.styles.ActionHints.Render("  esc cancel"))
	return b.String()
}

// helpBindings reuses idle bindings; the editor's own controls are inline.
func (configEditorMode) helpBindings(m Model) []binding {
	return idleMode{}.helpBindings(m)
}
