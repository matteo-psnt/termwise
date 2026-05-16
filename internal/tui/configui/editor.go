package configui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// ---------------------------------------------------------------------------
// Row types
// ---------------------------------------------------------------------------

type editorRowKind int

const (
	rowSectionHeader editorRowKind = iota
	rowProvHeader
	rowProvModel
	rowProvAuth
	rowAddProvider
	rowSetting // driven by editorSettings table; settingIdx identifies which one
)

type editorRow struct {
	kind       editorRowKind
	provider   string
	label      string
	settingIdx int // only meaningful when kind == rowSetting
}

// settingDef describes a single settings row declaratively.
// Adding a new setting requires only a new entry here — nothing else changes.
type settingDef struct {
	label    string
	hint     string
	getValue func(cfg config.FileConfig) string
	activate func(m editorModel) (editorModel, tea.Cmd)
}

var editorSettings = []settingDef{
	{
		label: "Keybinding",
		hint:  "enter edit   ↑/↓ navigate   q quit",
		getValue: func(cfg config.FileConfig) string {
			kb := cfg.Shell.Keybinding
			if kb == "" {
				kb = "^T"
			}
			return keybinding.Label(kb)
		},
		activate: func(m editorModel) (editorModel, tea.Cmd) {
			kb := m.cfg.Shell.Keybinding
			if kb == "" {
				kb = "^T"
			}
			kbm := newKeybindingCapture(kb, m.styles)
			m.keybinding = &kbm
			return m, nil
		},
	},
	{
		label: "Theme",
		hint:  "enter edit   ↑/↓ navigate   q quit",
		getValue: func(cfg config.FileConfig) string {
			return theme.Label(cfg.UI.Theme)
		},
		activate: func(m editorModel) (editorModel, tea.Cmd) {
			tp := newThemePicker(m.cfg.UI.Theme, m.r, m.styles)
			m.themePicker = &tp
			return m, nil
		},
	},
	{
		label: "LLM judge",
		hint:  "enter toggle   ↑/↓ navigate   q quit",
		getValue: func(cfg config.FileConfig) string {
			if cfg.Policies.Bash.LLMJudge {
				return "on"
			}
			return "off"
		},
		activate: func(m editorModel) (editorModel, tea.Cmd) {
			m.cfg.Policies.Bash.LLMJudge = !m.cfg.Policies.Bash.LLMJudge
			return m, nil
		},
	},
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type editorModel struct {
	styles configStyles
	r      *lipgloss.Renderer
	width  int
	height int
	spin   spinner.Model

	cfgPath string
	cfg     config.FileConfig

	rows   []editorRow
	cursor int

	// Connectivity check for the active provider.
	connChecked bool
	connOK      bool
	connErr     string

	// Sub-models — at most one non-nil at a time.
	modelPicker *modelPickerModel
	authEditor  *authEditorModel
	keybinding  *keybindingModel
	themePicker *themePickerModel
	addWizard   *wizardModel

	err error
}

func newEditorModel(cfgPath string, cfg config.FileConfig, r *lipgloss.Renderer) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	m := editorModel{
		styles:  newStylesForTheme(r, cfg.UI.Theme),
		r:       r,
		spin:    sp,
		cfgPath: cfgPath,
		cfg:     cfg,
	}
	m.buildRows()
	return m
}

// ---------------------------------------------------------------------------
// Row list management
// ---------------------------------------------------------------------------

func (m *editorModel) buildRows() {
	m.rows = []editorRow{{kind: rowSectionHeader, label: "Providers"}}
	for _, name := range m.sortedProviders() {
		m.rows = append(m.rows,
			editorRow{kind: rowProvHeader, provider: name},
			editorRow{kind: rowProvModel, provider: name},
			editorRow{kind: rowProvAuth, provider: name},
		)
	}
	if len(m.cfg.Providers) < len(config.ProviderInfos()) {
		m.rows = append(m.rows, editorRow{kind: rowAddProvider})
	}
	m.rows = append(m.rows, editorRow{kind: rowSectionHeader, label: "Settings"})
	for i := range editorSettings {
		m.rows = append(m.rows, editorRow{kind: rowSetting, settingIdx: i})
	}
	m.snapCursor()
}

func (m *editorModel) snapCursor() {
	for m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowSectionHeader {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m editorModel) moveCursor(delta int) editorModel {
	next := m.cursor + delta
	for next >= 0 && next < len(m.rows) {
		if m.rows[next].kind != rowSectionHeader {
			m.cursor = next
			return m
		}
		next += delta
	}
	return m
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func (m editorModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
}

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
			m.cfg.Shell.Keybinding = kb.result
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
			m.cfg.UI.Theme = theme.Normalize(tp.result)
			m.styles = newStylesForTheme(m.r, tp.result)
		} else {
			m.styles = newStylesForTheme(m.r, tp.prev)
		}
	} else if tp.PreviewName() != prevPreview {
		m.styles = newStylesForTheme(m.r, tp.PreviewName())
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
	case "up", "k":
		return m.moveCursor(-1), nil
	case "down", "j":
		return m.moveCursor(1), nil
	case "enter", " ":
		return m.activateRow()
	}
	return m, nil
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
		wiz := newWizardModel(m.r, theme.Normalize(m.cfg.UI.Theme))
		wiz.exclude = exclude
		m.addWizard = &wiz
		return m, m.addWizard.Init()

	case rowSetting:
		updated, cmd := editorSettings[row.settingIdx].activate(m)
		return updated, cmd
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (m editorModel) saveAndQuit() (tea.Model, tea.Cmd) {
	if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
		m.err = err
	}
	return m, tea.Quit
}

func (m *editorModel) resetConnectivity() {
	m.connChecked = false
	m.connOK = false
	m.connErr = ""
}

func (m editorModel) checkConnectivityCmd() tea.Cmd {
	providerName := m.cfg.SelectedProvider
	pc, ok := m.cfg.Providers[providerName]
	if !ok {
		return func() tea.Msg {
			return editorConnMsg{ok: false, err: fmt.Errorf("provider not configured")}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := config.CheckProviderConnectivity(ctx, providerName, pc); err != nil {
			return editorConnMsg{ok: false, err: err}
		}
		return editorConnMsg{ok: true}
	}
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func (m editorModel) sortedProviders() []string {
	names := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m editorModel) connIndicator() string {
	if !m.connChecked {
		return m.styles.Spinner.Render(m.spin.View())
	}
	if m.connOK {
		return m.styles.Success.Render("●")
	}
	return m.styles.Error.Render("●")
}

func describeAuth(providerName string, pc config.ProviderConfig) string {
	if providerName == "ollama" {
		if pc.BaseURL == "" {
			return "base URL · (default)"
		}
		return "base URL · " + pc.BaseURL
	}
	method := pc.Auth
	if method == "" {
		method = "env"
	}
	switch method {
	case "env":
		v := pc.EnvVar
		if v == "" {
			v = "(default)"
		}
		return "env · " + v
	case "cmd":
		v := pc.APIKeyCmd
		if v == "" {
			v = "(not set)"
		}
		if len(v) > 40 {
			v = v[:37] + "..."
		}
		return "cmd · " + v
	case "keychain":
		v := pc.KeychainEntry
		if v == "" {
			v = "(not set)"
		}
		return "keychain · " + v
	default:
		return method
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m editorModel) View() string {
	inner := m.renderInner()
	box := m.styles.Outer.Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m editorModel) renderInner() string {
	header := m.styles.Title.Render("termwise config") + "\n\n"
	switch {
	case m.modelPicker != nil:
		return header + m.modelPicker.View()
	case m.authEditor != nil:
		return header + m.authEditor.View()
	case m.keybinding != nil:
		return header + m.keybinding.View()
	case m.themePicker != nil:
		return header + m.themePicker.View()
	case m.addWizard != nil:
		return m.addWizard.renderInner()
	default:
		return m.renderNormal()
	}
}

func (m editorModel) renderNormal() string {
	var b strings.Builder

	for i, row := range m.rows {
		focused := i == m.cursor

		switch row.kind {
		case rowSectionHeader:
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("  " + m.styles.Title.Render(row.label) + "\n")

		case rowProvHeader:
			prefix := m.rowPrefix(focused)
			indicator := ""
			if row.provider == m.cfg.SelectedProvider {
				indicator = " " + m.connIndicator()
			}
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(row.provider) + indicator + "\n")
			} else {
				b.WriteString(prefix + row.provider + indicator + "\n")
			}

		case rowProvModel:
			val := m.cfg.Providers[row.provider].Model
			if val == "" {
				val = "(none)"
			}
			m.renderRow(&b, focused, "     Model   ", val)

		case rowProvAuth:
			m.renderRow(&b, focused, "     Auth    ", describeAuth(row.provider, m.cfg.Providers[row.provider]))

		case rowAddProvider:
			prefix := m.rowPrefix(focused)
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render("+ Add provider") + "\n")
			} else {
				b.WriteString(prefix + m.styles.Dim.Render("+ Add provider") + "\n")
			}

		case rowSetting:
			def := editorSettings[row.settingIdx]
			m.renderRow(&b, focused, fmt.Sprintf("%-13s", def.label), def.getValue(m.cfg))
		}
	}

	b.WriteString("\n")
	if len(m.rows) > 0 && m.cursor < len(m.rows) {
		switch m.rows[m.cursor].kind {
		case rowProvHeader:
			b.WriteString(m.styles.Dim.Render("enter set active   ↑/↓ navigate   q quit"))
		case rowProvModel:
			b.WriteString(m.styles.Dim.Render("enter pick model   ↑/↓ navigate   q quit"))
		case rowProvAuth:
			b.WriteString(m.styles.Dim.Render("enter edit auth   ↑/↓ navigate   q quit"))
		case rowAddProvider:
			b.WriteString(m.styles.Dim.Render("enter add provider   ↑/↓ navigate   q quit"))
		case rowSetting:
			b.WriteString(m.styles.Dim.Render(editorSettings[m.rows[m.cursor].settingIdx].hint))
		default:
			b.WriteString(m.styles.Dim.Render("enter edit   ↑/↓ navigate   q quit"))
		}
	}

	return b.String()
}

func (m editorModel) rowPrefix(focused bool) string {
	if focused {
		return m.styles.Selected.Render("▶ ")
	}
	return "  "
}

// renderRow renders a labeled value row. label should already be padded/formatted
// for display (e.g. "     Model   " for sub-rows, fmt.Sprintf("%-13s", ...) for settings).
func (m editorModel) renderRow(b *strings.Builder, focused bool, label, value string) {
	prefix := m.rowPrefix(focused)
	if focused {
		b.WriteString(prefix + m.styles.Selected.Render(label+value) + "\n")
	} else {
		b.WriteString(prefix + m.styles.Dim.Render(label) + value + "\n")
	}
}
