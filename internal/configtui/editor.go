package configtui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"

	_ "github.com/matteo-psnt/termwise/internal/ai/anthropic"
	_ "github.com/matteo-psnt/termwise/internal/ai/openaicompat"
)

// ---------------------------------------------------------------------------
// State & row enums
// ---------------------------------------------------------------------------

type editorState int

const (
	editorNormal          editorState = iota
	editorPickModel                   // inline model picker
	editorPickAuth                    // picking auth method
	editorEnterAuthValue              // text entry for auth value
	editorAuthWorking                 // verifying credentials
	editorEnterKeybinding             // key-capture mode
	editorPickTheme                   // inline theme picker
	editorAddProvider                 // embedded add-provider wizard
)

type editorRowKind int

const (
	rowSectionHeader editorRowKind = iota // non-interactive visual separator
	rowProvHeader                         // provider name — enter sets active
	rowProvModel                          // model sub-row — enter opens picker
	rowProvAuth                           // auth sub-row  — enter opens editor
	rowAddProvider                        // "+ Add provider"
	rowKeybinding                         // keybinding setting
	rowTheme                              // theme setting
)

type editorRow struct {
	kind     editorRowKind
	provider string // set for rowProvHeader / rowProvModel / rowProvAuth
	label    string // set for rowSectionHeader
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
	cfg     config.Config

	state  editorState
	rows   []editorRow
	cursor int

	// Connectivity check for the active provider.
	connChecked bool
	connOK      bool
	connErr     string

	// Model picker sub-state.
	pickingProvider string
	modelList       []ai.Model
	modelCursor     int
	modelLoading    bool
	modelErr        string

	// Auth editing sub-flow.
	editingAuthProvider string
	editingAuthMethod   string
	authFromPicker      bool // true when reached via auth picker (esc goes back there)
	authMethodCursor    int
	authInput           textinput.Model
	authInputLabel      string
	authInputHint       string
	authInputFallback   string
	authErr             string

	// Keybinding capture.
	kbCaptured   string
	kbConfirming bool

	// Theme picker.
	themeCursor int
	themePrev   string

	// Add-provider wizard (embedded).
	addWizard *wizardModel

	err error
}

func newEditorModel(cfgPath string, cfg config.Config, r *lipgloss.Renderer) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	authInput := textinput.New()
	authInput.CharLimit = 256

	m := editorModel{
		styles:    newStylesForTheme(r, cfg.TUI.Theme),
		r:         r,
		spin:      sp,
		cfgPath:   cfgPath,
		cfg:       cfg,
		authInput: authInput,
	}
	m.buildRows()
	return m
}

// buildRows reconstructs the navigable row list from the current config.
// Section-header rows are skipped by the cursor.
func (m *editorModel) buildRows() {
	m.rows = []editorRow{
		{kind: rowSectionHeader, label: "Providers"},
	}
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
	m.rows = append(m.rows,
		editorRow{kind: rowSectionHeader, label: "Settings"},
		editorRow{kind: rowKeybinding},
		editorRow{kind: rowTheme},
	)
	// Ensure the cursor lands on a navigable row.
	m.snapCursor()
}

// snapCursor advances the cursor forward until it rests on a navigable row.
func (m *editorModel) snapCursor() {
	for m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowSectionHeader {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

// moveCursor moves the cursor by delta steps, skipping section headers.
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
	// The add-provider wizard takes priority.
	if m.state == editorAddProvider && m.addWizard != nil {
		newModel, cmd := m.addWizard.Update(msg)
		newWiz := newModel.(wizardModel)
		m.addWizard = &newWiz

		if newWiz.Result != nil {
			for name, pc := range newWiz.Result.Providers {
				m.cfg.SetProvider(name, pc)
			}
			m.addWizard = nil
			m.state = editorNormal
			m.buildRows()
			m.resetConnectivity()
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}
		if newWiz.done {
			m.addWizard = nil
			m.state = editorNormal
			return m, nil
		}
		return m, cmd
	}

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

	case editorModelsMsg:
		m.modelLoading = false
		if msg.err != nil {
			m.modelErr = msg.err.Error()
			m.modelList = nil
			return m, nil
		}
		m.modelList = msg.models
		m.modelCursor = 0
		current := m.cfg.Providers[m.pickingProvider].Model
		for i, mod := range m.modelList {
			if mod.ID == current {
				m.modelCursor = i
				break
			}
		}
		return m, nil

	case editorAuthMsg:
		if m.state != editorAuthWorking {
			return m, nil
		}
		if !msg.ok {
			m.authErr = msg.err.Error()
			m.authInput.Focus()
			m.state = editorEnterAuthValue
			return m, nil
		}
		var pc config.ProviderConfig
		if m.editingAuthMethod == "keychain" {
			pc = config.ProviderConfig{
				AuthMethod:    "keychain",
				KeychainEntry: config.DefaultKeychainEntry(m.editingAuthProvider),
			}
		} else {
			pc = buildProviderConfigFromAuthInput(
				m.editingAuthProvider, m.editingAuthMethod,
				m.authInput.Value(), m.authInputFallback,
			)
		}
		existing := m.cfg.Providers[m.editingAuthProvider]
		pc.Model = existing.Model
		m.cfg.SetProvider(m.editingAuthProvider, pc)
		m.state = editorNormal
		m.resetConnectivity()
		return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m editorModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case editorPickModel:
		return m.handleModelPickerKey(msg)
	case editorPickAuth:
		return m.handleAuthPickerKey(msg)
	case editorEnterAuthValue:
		return m.handleAuthEnterKey(msg)
	case editorEnterKeybinding:
		return m.handleKbKey(msg)
	case editorPickTheme:
		return m.handleThemePickerKey(msg)
	}
	return m.handleNormalKey(msg)
}

// ---------------------------------------------------------------------------
// Key handlers — normal state
// ---------------------------------------------------------------------------

func (m editorModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "q":
		if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
			m.err = err
		}
		return m, tea.Quit

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
		if row.provider != m.cfg.ActiveProvider {
			m.cfg.ActiveProvider = row.provider
			m.resetConnectivity()
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}

	case rowProvModel:
		return m.openModelPicker(row.provider)

	case rowProvAuth:
		m.editingAuthProvider = row.provider
		if row.provider == "ollama" {
			m.editingAuthMethod = "env"
			m.authFromPicker = false
			m.setupAuthEnterValue()
			m.state = editorEnterAuthValue
			return m, nil
		}
		pc := m.cfg.Providers[row.provider]
		cur := pc.AuthMethod
		if cur == "" {
			cur = "env"
		}
		m.authMethodCursor = 0
		for i, am := range authMethods {
			if am.id == cur {
				m.authMethodCursor = i
				break
			}
		}
		m.state = editorPickAuth

	case rowAddProvider:
		exclude := make(map[string]bool, len(m.cfg.Providers))
		for name := range m.cfg.Providers {
			exclude[name] = true
		}
		wiz := newWizardModel(m.r, theme.Normalize(m.cfg.TUI.Theme))
		wiz.exclude = exclude
		m.addWizard = &wiz
		m.state = editorAddProvider
		return m, m.addWizard.Init()

	case rowKeybinding:
		m.kbCaptured = ""
		m.kbConfirming = false
		m.state = editorEnterKeybinding

	case rowTheme:
		m.themePrev = m.cfg.TUI.Theme
		m.themeCursor = themeIndex(m.themePrev)
		m.applyTheme(theme.All()[m.themeCursor].Name)
		m.state = editorPickTheme
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Key handlers — sub-states
// ---------------------------------------------------------------------------

func (m editorModel) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
			m.err = err
		}
		return m, tea.Quit
	case "esc", "b":
		m.state = editorNormal
		m.modelList = nil
		m.modelErr = ""
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "down", "j":
		if m.modelCursor < len(m.modelList)-1 {
			m.modelCursor++
		}
	case "enter", " ":
		if len(m.modelList) == 0 {
			break
		}
		selected := m.modelList[m.modelCursor].ID
		pc := m.cfg.Providers[m.pickingProvider]
		pc.Model = selected
		m.cfg.SetProvider(m.pickingProvider, pc)
		m.state = editorNormal
		m.modelList = nil
		m.modelErr = ""
	}
	return m, nil
}

func (m editorModel) handleAuthPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
			m.err = err
		}
		return m, tea.Quit
	case "esc", "b":
		m.state = editorNormal
	case "up", "k":
		if m.authMethodCursor > 0 {
			m.authMethodCursor--
		}
	case "down", "j":
		if m.authMethodCursor < len(authMethods)-1 {
			m.authMethodCursor++
		}
	case "enter", " ":
		selected := authMethods[m.authMethodCursor].id
		cur := m.cfg.Providers[m.editingAuthProvider].AuthMethod
		if cur == "" {
			cur = "env"
		}
		if selected == cur {
			if m.editingAuthProvider == m.cfg.ActiveProvider && m.connChecked && m.connOK {
				m.state = editorNormal
				return m, nil
			}
			m.editingAuthMethod = selected
			m.authFromPicker = true
			m.setupAuthEnterValue()
			if selected != "keychain" {
				m.authInput.Blur()
				m.state = editorAuthWorking
				return m, tea.Batch(m.spin.Tick, m.verifyAuthCmd())
			}
			m.state = editorEnterAuthValue
			return m, nil
		}
		m.editingAuthMethod = selected
		m.authFromPicker = true
		m.setupAuthEnterValue()
		m.state = editorEnterAuthValue
	}
	return m, nil
}

func (m editorModel) handleAuthEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.authInput.Blur()
		m.authErr = ""
		if m.authFromPicker {
			m.state = editorPickAuth
		} else {
			m.state = editorNormal
		}
	case "enter":
		val := strings.TrimSpace(m.authInput.Value())
		if val == "" && m.authInputFallback == "" {
			m.authErr = "value cannot be empty"
			return m, nil
		}
		m.authInput.Blur()
		m.authErr = ""
		m.state = editorAuthWorking
		return m, tea.Batch(m.spin.Tick, m.verifyAuthCmd())
	default:
		var cmd tea.Cmd
		m.authInput, cmd = m.authInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m editorModel) handleKbKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.kbConfirming {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.kbCaptured = ""
			m.kbConfirming = false
			m.state = editorNormal
		case "enter":
			m.cfg.Shell.Keybinding = m.kbCaptured
			m.kbCaptured = ""
			m.kbConfirming = false
			m.state = editorNormal
		default:
			if binding, ok := keyMsgToZsh(msg); ok {
				m.kbCaptured = binding
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.state = editorNormal
	default:
		if binding, ok := keyMsgToZsh(msg); ok {
			m.kbCaptured = binding
			m.kbConfirming = true
		}
	}
	return m, nil
}

func (m editorModel) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	palettes := theme.All()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
			m.err = err
		}
		return m, tea.Quit
	case "esc", "b":
		m.applyTheme(m.themePrev)
		m.state = editorNormal
	case "up", "k":
		if m.themeCursor > 0 {
			m.themeCursor--
			m.applyTheme(palettes[m.themeCursor].Name)
		}
	case "down", "j":
		if m.themeCursor < len(palettes)-1 {
			m.themeCursor++
			m.applyTheme(palettes[m.themeCursor].Name)
		}
	case "enter", " ":
		m.applyTheme(palettes[m.themeCursor].Name)
		m.state = editorNormal
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Auth helpers
// ---------------------------------------------------------------------------

func (m *editorModel) setupAuthEnterValue() {
	provider := m.editingAuthProvider
	method := m.editingAuthMethod
	pc := m.cfg.Providers[provider]

	m.authInput.EchoMode = textinput.EchoNormal

	if provider == "ollama" {
		m.authInputLabel = "Ollama base URL"
		m.authInputHint = "leave blank for default (" + defaultOllamaBaseURL + ")"
		m.authInputFallback = defaultOllamaBaseURL
		m.authInput.Placeholder = defaultOllamaBaseURL
		m.authInput.SetValue(pc.BaseURL)
		m.authErr = ""
		m.authInput.Focus()
		return
	}

	switch method {
	case "env":
		def := config.DefaultEnvVar(provider)
		m.authInputLabel = "Environment variable name"
		m.authInputHint = "the env var that holds your API key"
		m.authInputFallback = def
		m.authInput.Placeholder = def
		m.authInput.SetValue(pc.EnvVar)
	case "cmd":
		m.authInputLabel = "Shell command"
		m.authInputHint = "command whose stdout is the API key"
		m.authInputFallback = ""
		m.authInput.Placeholder = "op read op://vault/item/field"
		m.authInput.SetValue(pc.APIKeyCmd)
	case "keychain":
		m.authInputLabel = "API key"
		m.authInputHint = "will be stored securely in macOS Keychain"
		m.authInputFallback = ""
		m.authInput.EchoMode = textinput.EchoPassword
		m.authInput.Placeholder = "sk-..."
		m.authInput.SetValue("")
	}
	m.authErr = ""
	m.authInput.Focus()
}

func (m *editorModel) resetConnectivity() {
	m.connChecked = false
	m.connOK = false
	m.connErr = ""
}

func (m *editorModel) applyTheme(name string) {
	name = theme.Normalize(name)
	m.cfg.TUI.Theme = name
	m.styles = newStylesForTheme(m.r, name)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (m editorModel) checkConnectivityCmd() tea.Cmd {
	providerName := m.cfg.ActiveProvider
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

func (m editorModel) openModelPicker(providerName string) (tea.Model, tea.Cmd) {
	m.state = editorPickModel
	m.pickingProvider = providerName
	m.modelList = nil
	m.modelCursor = 0
	m.modelLoading = true
	m.modelErr = ""
	return m, tea.Batch(m.spin.Tick, m.fetchModelsCmd(providerName))
}

func (m editorModel) fetchModelsCmd(providerName string) tea.Cmd {
	pc, ok := m.cfg.Providers[providerName]
	if !ok {
		return func() tea.Msg {
			return editorModelsMsg{providerName: providerName, err: fmt.Errorf("provider not configured")}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := config.ListProviderModels(ctx, providerName, pc)
		return editorModelsMsg{providerName: providerName, models: models, err: err}
	}
}

func (m editorModel) verifyAuthCmd() tea.Cmd {
	provider := m.editingAuthProvider
	method := m.editingAuthMethod
	inputVal := m.authInput.Value()
	fallback := m.authInputFallback

	return func() tea.Msg {
		var pc config.ProviderConfig
		if method == "keychain" {
			if err := config.StoreKeychain(provider, inputVal); err != nil {
				return editorAuthMsg{providerName: provider, ok: false, err: fmt.Errorf("keychain write: %w", err)}
			}
			pc = config.ProviderConfig{
				AuthMethod:    "keychain",
				KeychainEntry: config.DefaultKeychainEntry(provider),
			}
		} else {
			pc = buildProviderConfigFromAuthInput(provider, method, inputVal, fallback)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := config.CheckProviderConnectivity(ctx, provider, pc); err != nil {
			return editorAuthMsg{providerName: provider, ok: false, err: err}
		}
		return editorAuthMsg{providerName: provider, ok: true}
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

func themeIndex(name string) int {
	palettes := theme.All()
	name = theme.Normalize(name)
	for i, p := range palettes {
		if p.Name == name {
			return i
		}
	}
	return 0
}

func describeAuth(provider string, pc config.ProviderConfig) string {
	if provider == "ollama" {
		if pc.BaseURL == "" {
			return "base URL · (default)"
		}
		return "base URL · " + pc.BaseURL
	}
	method := pc.AuthMethod
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

func paletteSwatches(r *lipgloss.Renderer, palette theme.Palette) string {
	colors := []lipgloss.AdaptiveColor{
		palette.Accent,
		palette.Success,
		palette.Error,
		palette.Border,
	}
	var out strings.Builder
	for _, c := range colors {
		out.WriteString(r.NewStyle().Foreground(c).Render("●"))
		out.WriteString(" ")
	}
	return strings.TrimSpace(out.String())
}

func renderThemePreview(r *lipgloss.Renderer, palette theme.Palette) string {
	accent := r.NewStyle().Foreground(palette.Accent).Bold(true)
	success := r.NewStyle().Foreground(palette.Success)
	errorStyle := r.NewStyle().Foreground(palette.Error)
	muted := r.NewStyle().Foreground(palette.Muted)
	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(palette.Border).
		Padding(0, 1)

	var b strings.Builder
	b.WriteString(accent.Render("Preview: ") + palette.Label + "\n")
	b.WriteString("tw: ")
	b.WriteString(accent.Render("rg \"theme\" internal") + "\n")
	b.WriteString(success.Render("connected") + "  ")
	b.WriteString(errorStyle.Render("error") + "  ")
	b.WriteString(muted.Render("footer · 12,481 tok"))
	return box.Render(b.String())
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

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m editorModel) View() string {
	inner := m.renderInner()
	box := m.styles.Outer.Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m editorModel) renderInner() string {
	switch m.state {
	case editorPickModel:
		return m.renderModelPicker()
	case editorPickAuth:
		return m.renderAuthPicker()
	case editorEnterAuthValue:
		return m.renderEnterAuthValue()
	case editorAuthWorking:
		return m.renderAuthWorking()
	case editorEnterKeybinding:
		return m.renderKeybinding()
	case editorPickTheme:
		return m.renderThemePicker()
	case editorAddProvider:
		if m.addWizard != nil {
			return m.addWizard.renderInner()
		}
		return m.renderNormal()
	default:
		return m.renderNormal()
	}
}

// ---------------------------------------------------------------------------
// Render — normal view
// ---------------------------------------------------------------------------

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
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			indicator := ""
			if row.provider == m.cfg.ActiveProvider {
				indicator = " " + m.connIndicator()
			}
			if focused {
				b.WriteString(cur + m.styles.Selected.Render(row.provider) + indicator + "\n")
			} else {
				b.WriteString(cur + row.provider + indicator + "\n")
			}

		case rowProvModel:
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			val := m.cfg.Providers[row.provider].Model
			if val == "" {
				val = "(none)"
			}
			if focused {
				b.WriteString(cur + m.styles.Selected.Render("     Model   "+val) + "\n")
			} else {
				b.WriteString(cur + m.styles.Dim.Render("     Model   ") + val + "\n")
			}

		case rowProvAuth:
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			val := describeAuth(row.provider, m.cfg.Providers[row.provider])
			if focused {
				b.WriteString(cur + m.styles.Selected.Render("     Auth    "+val) + "\n")
			} else {
				b.WriteString(cur + m.styles.Dim.Render("     Auth    ") + val + "\n")
			}

		case rowAddProvider:
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			line := "+ Add provider"
			if focused {
				b.WriteString(cur + m.styles.Selected.Render(line) + "\n")
			} else {
				b.WriteString(cur + m.styles.Dim.Render(line) + "\n")
			}

		case rowKeybinding:
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			kb := m.cfg.Shell.Keybinding
			if kb == "" {
				kb = "^T"
			}
			line := fmt.Sprintf("%-13s%s", "Keybinding", zshToLabel(kb))
			if focused {
				b.WriteString(cur + m.styles.Selected.Render(line) + "\n")
			} else {
				b.WriteString(cur + m.styles.Dim.Render("Keybinding   ") + zshToLabel(kb) + "\n")
			}

		case rowTheme:
			cur := "  "
			if focused {
				cur = m.styles.Selected.Render("▶ ")
			}
			val := theme.Label(m.cfg.TUI.Theme)
			if focused {
				b.WriteString(cur + m.styles.Selected.Render(fmt.Sprintf("%-13s%s", "Theme", val)) + "\n")
			} else {
				b.WriteString(cur + m.styles.Dim.Render("Theme        ") + val + "\n")
			}
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
		default:
			b.WriteString(m.styles.Dim.Render("enter edit   ↑/↓ navigate   q quit"))
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Render — sub-views
// ---------------------------------------------------------------------------

func (m editorModel) renderModelPicker() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString("Model for " + m.styles.Title.Render(m.pickingProvider) + ":\n\n")

	if m.modelLoading && len(m.modelList) == 0 {
		b.WriteString(m.styles.Spinner.Render(m.spin.View()) + " Fetching models...")
		return b.String()
	}
	if m.modelErr != "" {
		b.WriteString(m.styles.Error.Render("Error: "+m.modelErr) + "\n\n")
		b.WriteString(m.styles.Dim.Render("esc back"))
		return b.String()
	}
	for i, mod := range m.modelList {
		if i == m.modelCursor {
			b.WriteString(m.styles.Selected.Render("▶ "+mod.ID) + "\n")
		} else {
			b.WriteString("  " + mod.ID + "\n")
		}
	}
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))
	return b.String()
}

func (m editorModel) renderAuthPicker() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString("Auth method for " + m.styles.Title.Render(m.editingAuthProvider) + ":\n\n")

	cur := m.cfg.Providers[m.editingAuthProvider].AuthMethod
	if cur == "" {
		cur = "env"
	}
	for i, a := range authMethods {
		current := ""
		if a.id == cur {
			current = " " + m.styles.Dim.Render("(current)")
		}
		if i == m.authMethodCursor {
			b.WriteString(m.styles.Selected.Render("▶ "+a.label) + current + "\n")
		} else {
			b.WriteString("  " + a.label + current + "\n")
		}
	}
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))
	return b.String()
}

func (m editorModel) renderEnterAuthValue() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString(m.authInputLabel + ":\n\n")
	b.WriteString(m.authInput.View() + "\n")
	if m.authInputHint != "" {
		b.WriteString("\n" + m.styles.Hint.Render(m.authInputHint))
	}
	if m.authErr != "" {
		b.WriteString("\n\n" + m.styles.Error.Render("Error: "+m.authErr))
	}
	b.WriteString("\n\n" + m.styles.Dim.Render("enter verify & save   esc back"))
	return b.String()
}

func (m editorModel) renderAuthWorking() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString(m.styles.Spinner.Render(m.spin.View()) + " Verifying with " + m.editingAuthProvider + "...")
	return b.String()
}

func (m editorModel) renderKeybinding() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString("Shell keybinding:\n\n")

	if m.kbConfirming {
		b.WriteString("Captured: " + m.styles.Selected.Render(zshToLabel(m.kbCaptured)) + "\n\n")
		b.WriteString(m.styles.Hint.Render(`takes effect in new terminals, or run: eval "$(termwise init zsh)"`))
		b.WriteString("\n\n" + m.styles.Dim.Render("enter confirm   any key retry   esc cancel"))
	} else {
		kb := m.cfg.Shell.Keybinding
		if kb == "" {
			kb = "^T"
		}
		b.WriteString("Current: " + m.styles.Normal.Render(zshToLabel(kb)) + "\n\n")
		b.WriteString("Press a key combination...\n")
		b.WriteString("\n" + m.styles.Dim.Render("esc cancel"))
	}
	return b.String()
}

func (m editorModel) renderThemePicker() string {
	palettes := theme.All()
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString("Theme palette:\n\n")

	for i, p := range palettes {
		line := paletteSwatches(m.r, p) + " " + p.Label
		if p.Name == theme.Normalize(m.cfg.TUI.Theme) {
			line += " " + m.styles.Dim.Render("(selected)")
		}
		if i == m.themeCursor {
			b.WriteString(m.styles.Selected.Render("▶ ") + line + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(renderThemePreview(m.r, palettes[m.themeCursor]))
	b.WriteString("\n\n" + m.styles.Dim.Render("↑/↓ preview   enter select   esc cancel   q quit"))
	return b.String()
}
