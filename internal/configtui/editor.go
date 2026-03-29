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
	editorPickModel                   // inline model picker open
	editorPickAuth                    // picking auth method for a provider
	editorPickTheme                   // picking a TUI theme preset
	editorEnterAuthValue              // entering auth value (env var / cmd / keychain)
	editorAuthWorking                 // verifying auth credentials
	editorEnterKeybinding             // editing shell keybinding
	editorAddProvider                 // wizard sub-flow
)

type editorRowKind int

const (
	rowActiveProvider editorRowKind = iota
	rowModel
	rowAuth
	rowKeybinding
	rowTheme
	rowAddProvider
	rowSave
	rowCancel
)

// editorRow is one navigable row in the editor.
type editorRow struct {
	kind     editorRowKind
	provider string // for rowModel and rowAuth
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

	// Navigation
	state  editorState
	rows   []editorRow
	cursor int

	// Connectivity check (for active provider)
	connChecked bool
	connOK      bool
	connErr     string

	// Model picker sub-state
	pickingProvider string
	modelList       []ai.Model
	modelCursor     int
	modelLoading    bool
	modelErr        string

	// Auth editing sub-flow
	editingAuthProvider string
	editingAuthMethod   string
	authMethodCursor    int
	authInput           textinput.Model
	authInputLabel      string
	authInputHint       string
	authInputFallback   string
	authErr             string

	// Keybinding editing
	kbInput textinput.Model

	// Theme picker
	themeCursor int
	themePrev   string

	// Add provider — embedded wizard
	addWizard *wizardModel

	// Exit
	saved bool
	quit  bool
	err   error
}

func newEditorModel(cfgPath string, cfg config.Config, r *lipgloss.Renderer) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	authInput := textinput.New()
	authInput.CharLimit = 256

	kbInput := textinput.New()
	kbInput.CharLimit = 32

	m := editorModel{
		styles:    newStylesForTheme(r, cfg.TUI.Theme),
		r:         r,
		spin:      sp,
		cfgPath:   cfgPath,
		cfg:       cfg,
		authInput: authInput,
		kbInput:   kbInput,
	}
	m.buildRows()
	return m
}

// buildRows constructs the flat list of navigable rows from cfg.
func (m *editorModel) buildRows() {
	m.rows = []editorRow{
		{kind: rowActiveProvider},
	}
	for _, name := range m.sortedProviders() {
		m.rows = append(m.rows,
			editorRow{kind: rowModel, provider: name},
			editorRow{kind: rowAuth, provider: name},
		)
	}
	m.rows = append(m.rows,
		editorRow{kind: rowKeybinding},
		editorRow{kind: rowTheme},
		editorRow{kind: rowAddProvider},
		editorRow{kind: rowSave},
		editorRow{kind: rowCancel},
	)
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
	// When the add-provider wizard is active, delegate all messages to it.
	if m.state == editorAddProvider && m.addWizard != nil {
		newModel, cmd := m.addWizard.Update(msg)
		newWiz := newModel.(wizardModel)
		m.addWizard = &newWiz

		if newWiz.Result != nil {
			// Wizard completed — merge new provider into our config.
			for name, pc := range newWiz.Result.Providers {
				m.cfg.SetProvider(name, pc)
			}
			m.addWizard = nil
			m.state = editorNormal
			m.buildRows()
			m.connChecked = false
			m.connOK = false
			m.connErr = ""
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}
		if newWiz.done {
			// User quit the wizard without completing.
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
		// Success — build updated ProviderConfig preserving model.
		var pc config.ProviderConfig
		if m.editingAuthMethod == "keychain" {
			pc = config.ProviderConfig{
				AuthMethod:    "keychain",
				KeychainEntry: config.DefaultKeychainEntry(m.editingAuthProvider),
			}
		} else {
			pc = buildPCFromAuthInput(m.editingAuthProvider, m.editingAuthMethod, m.authInput.Value(), m.authInputFallback)
		}
		existing := m.cfg.Providers[m.editingAuthProvider]
		pc.Model = existing.Model
		m.cfg.SetProvider(m.editingAuthProvider, pc)
		m.state = editorNormal
		m.connChecked = false
		m.connOK = false
		m.connErr = ""
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
	case editorPickTheme:
		return m.handleThemePickerKey(msg)
	case editorEnterAuthValue:
		return m.handleAuthEnterKey(msg)
	case editorEnterKeybinding:
		return m.handleKbEnterKey(msg)
	}
	return m.handleNormalKey(msg)
}

func (m editorModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}

	case "enter", " ":
		row := m.rows[m.cursor]
		switch row.kind {

		case rowActiveProvider:
			providers := m.sortedProviders()
			if len(providers) == 0 {
				return m, nil
			}
			idx := indexOf(providers, m.cfg.ActiveProvider)
			m.cfg.ActiveProvider = providers[(idx+1)%len(providers)]
			m.connChecked = false
			m.connOK = false
			m.connErr = ""
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())

		case rowModel:
			return m.openModelPicker(row.provider)

		case rowAuth:
			m.editingAuthProvider = row.provider
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

		case rowKeybinding:
			kb := m.cfg.Shell.Keybinding
			if kb == "" {
				kb = "^T"
			}
			m.kbInput.SetValue(kb)
			m.kbInput.Focus()
			m.state = editorEnterKeybinding

		case rowTheme:
			m.themePrev = m.cfg.TUI.Theme
			m.themeCursor = themeIndex(m.themePrev)
			m.applyTheme(theme.All()[m.themeCursor].Name)
			m.state = editorPickTheme

		case rowAddProvider:
			wiz := newWizardModel(m.r, theme.Normalize(m.cfg.TUI.Theme))
			m.addWizard = &wiz
			m.state = editorAddProvider
			return m, m.addWizard.Init()

		case rowSave:
			if err := config.SaveConfig(m.cfgPath, m.cfg); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.saved = true
			return m, tea.Quit

		case rowCancel:
			m.quit = true
			return m, tea.Quit
		}

	case "left", "h":
		if row := m.rows[m.cursor]; row.kind == rowActiveProvider {
			providers := m.sortedProviders()
			if len(providers) == 0 {
				return m, nil
			}
			idx := indexOf(providers, m.cfg.ActiveProvider)
			m.cfg.ActiveProvider = providers[(idx-1+len(providers))%len(providers)]
			m.connChecked = false
			m.connOK = false
			m.connErr = ""
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}

	case "right", "l":
		if row := m.rows[m.cursor]; row.kind == rowActiveProvider {
			providers := m.sortedProviders()
			if len(providers) == 0 {
				return m, nil
			}
			idx := indexOf(providers, m.cfg.ActiveProvider)
			m.cfg.ActiveProvider = providers[(idx+1)%len(providers)]
			m.connChecked = false
			m.connOK = false
			m.connErr = ""
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
		}
	}

	return m, nil
}

func (m editorModel) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
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
			return m, nil
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
	case "ctrl+c", "q":
		m.quit = true
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
			// Same method as currently configured.
			// If this is the active provider and connectivity is already verified, nothing to do.
			if m.editingAuthProvider == m.cfg.ActiveProvider && m.connChecked && m.connOK {
				m.state = editorNormal
				return m, nil
			}
			// Keychain always asks for the key again (can't read it back to show it).
			// Other methods skip value entry and go straight to re-verification.
			m.editingAuthMethod = selected
			m.setupAuthEnterValue()
			if selected != "keychain" {
				m.authInput.Blur()
				m.state = editorAuthWorking
				return m, tea.Batch(m.spin.Tick, m.verifyAuthCmd())
			}
			m.state = editorEnterAuthValue
			return m, nil
		}

		// Different method — full flow.
		m.editingAuthMethod = selected
		m.setupAuthEnterValue()
		m.state = editorEnterAuthValue
	}
	return m, nil
}

func (m editorModel) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	palettes := theme.All()
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
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

func (m editorModel) handleAuthEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.authInput.Blur()
		m.authErr = ""
		m.state = editorPickAuth
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

func (m editorModel) handleKbEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.kbInput.Blur()
		m.state = editorNormal
	case "enter":
		m.kbInput.Blur()
		val := strings.TrimSpace(m.kbInput.Value())
		if val != "" {
			m.cfg.Shell.Keybinding = val
		}
		m.state = editorNormal
	default:
		var cmd tea.Cmd
		m.kbInput, cmd = m.kbInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Auth sub-flow helpers
// ---------------------------------------------------------------------------

// setupAuthEnterValue configures the auth textinput for the chosen method,
// pre-filling with the provider's current value if any.
func (m *editorModel) setupAuthEnterValue() {
	provider := m.editingAuthProvider
	method := m.editingAuthMethod
	pc := m.cfg.Providers[provider]

	m.authInput.EchoMode = textinput.EchoNormal // reset; keychain overrides below

	switch method {
	case "env":
		def := defaultEnvVar(provider)
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
		m.authInput.SetValue("") // never pre-fill a key
	}
	m.authErr = ""
	m.authInput.Focus()
}

// verifyAuthCmd verifies the entered credentials by calling ListModels.
func (m editorModel) verifyAuthCmd() tea.Cmd {
	provider := m.editingAuthProvider
	method := m.editingAuthMethod
	inputVal := m.authInput.Value()
	fallback := m.authInputFallback

	return func() tea.Msg {
		var pc config.ProviderConfig
		if method == "keychain" {
			// inputVal is the raw API key — write it to the keychain.
			if err := config.StoreKeychain(provider, inputVal); err != nil {
				return editorAuthMsg{providerName: provider, ok: false, err: fmt.Errorf("keychain write: %w", err)}
			}
			pc = config.ProviderConfig{
				AuthMethod:    "keychain",
				KeychainEntry: config.DefaultKeychainEntry(provider),
			}
		} else {
			pc = buildPCFromAuthInput(provider, method, inputVal, fallback)
		}
		auth, err := config.ResolveAuth(provider, pc)
		if err != nil {
			return editorAuthMsg{providerName: provider, ok: false, err: err}
		}
		p, err := ai.GetProvider(provider, ai.ProviderConfig{
			APIKey:  auth.APIKey,
			BaseURL: auth.BaseURL,
		})
		if err != nil {
			return editorAuthMsg{providerName: provider, ok: false, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = p.ListModels(ctx)
		if err != nil {
			return editorAuthMsg{providerName: provider, ok: false, err: err}
		}
		return editorAuthMsg{providerName: provider, ok: true}
	}
}

// buildPCFromAuthInput constructs a ProviderConfig from auth sub-flow state.
func buildPCFromAuthInput(provider, method, inputVal, fallback string) config.ProviderConfig {
	val := inputVal
	if val == "" {
		val = fallback
	}
	pc := config.ProviderConfig{AuthMethod: method}
	switch {
	case provider == "ollama":
		pc.AuthMethod = "env"
		if val != "" && val != "http://localhost:11434" {
			pc.BaseURL = val
		}
	case method == "env":
		pc.EnvVar = val
	case method == "cmd":
		pc.APIKeyCmd = val
	case method == "keychain":
		pc.KeychainEntry = val
	}
	return pc
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
		auth, err := config.ResolveAuth(providerName, pc)
		if err != nil {
			return editorConnMsg{ok: false, err: err}
		}
		p, err := ai.GetProvider(providerName, ai.ProviderConfig{
			APIKey:  auth.APIKey,
			BaseURL: auth.BaseURL,
		})
		if err != nil {
			return editorConnMsg{ok: false, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = p.ListModels(ctx)
		if err != nil {
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
		auth, err := config.ResolveAuth(providerName, pc)
		if err != nil {
			return editorModelsMsg{providerName: providerName, err: err}
		}
		p, err := ai.GetProvider(providerName, ai.ProviderConfig{
			APIKey:  auth.APIKey,
			BaseURL: auth.BaseURL,
		})
		if err != nil {
			return editorModelsMsg{providerName: providerName, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := p.ListModels(ctx)
		return editorModelsMsg{providerName: providerName, models: models, err: err}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m *editorModel) applyTheme(themeName string) {
	themeName = theme.Normalize(themeName)
	m.cfg.TUI.Theme = themeName
	m.styles = newStylesForTheme(m.r, themeName)
}

func (m editorModel) sortedProviders() []string {
	names := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return 0
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

// describeAuth returns a short human-readable summary of a provider's auth config.
func describeAuth(pc config.ProviderConfig) string {
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
	case editorPickTheme:
		return m.renderThemePicker()
	case editorEnterAuthValue:
		return m.renderEnterAuthValue()
	case editorAuthWorking:
		return m.renderAuthWorking()
	case editorEnterKeybinding:
		return m.renderEnterKeybinding()
	case editorAddProvider:
		if m.addWizard != nil {
			return m.addWizard.renderInner()
		}
		return m.renderNormal()
	default:
		return m.renderNormal()
	}
}

func (m editorModel) renderNormal() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")

	prevProvider := ""
	for i, row := range m.rows {
		focused := i == m.cursor
		prefix := "  "
		if focused {
			prefix = m.styles.Selected.Render("▶ ")
		}

		// Blank line between provider groups.
		if row.provider != "" && row.provider != prevProvider && prevProvider != "" {
			b.WriteString("\n")
		}
		prevProvider = row.provider

		switch row.kind {
		case rowActiveProvider:
			conn := m.connIndicator()
			label := "Active provider: "
			val := m.cfg.ActiveProvider
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+val) + " " + conn + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(val) + " " + conn + "\n")
			}

		case rowModel:
			pc := m.cfg.Providers[row.provider]
			val := pc.Model
			if val == "" {
				val = "(none)"
			}
			label := row.provider + " model:  "
			active := ""
			if row.provider == m.cfg.ActiveProvider {
				active = m.styles.Dim.Render(" (active)")
			}
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+val) + active + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(val) + active + "\n")
			}

		case rowAuth:
			pc := m.cfg.Providers[row.provider]
			label := row.provider + " auth:   "
			val := describeAuth(pc)
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+val) + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(val) + "\n")
			}

		case rowKeybinding:
			kb := m.cfg.Shell.Keybinding
			if kb == "" {
				kb = "^T"
			}
			label := "Keybinding:      "
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+kb) + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(kb) + "\n")
			}

		case rowTheme:
			label := "Theme:           "
			val := theme.Label(m.cfg.TUI.Theme)
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+val) + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(val) + "\n")
			}

		case rowAddProvider:
			line := "+ Add provider"
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(line) + "\n")
			} else {
				b.WriteString(prefix + m.styles.Dim.Render(line) + "\n")
			}

		case rowSave:
			line := "[Save]"
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(line) + "\n")
			} else {
				b.WriteString(prefix + m.styles.Normal.Render(line) + "\n")
			}

		case rowCancel:
			line := "[Cancel]"
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(line) + "\n")
			} else {
				b.WriteString(prefix + m.styles.Normal.Render(line) + "\n")
			}
		}
	}

	b.WriteString("\n")
	row := m.rows[m.cursor]
	switch row.kind {
	case rowActiveProvider:
		b.WriteString(m.styles.Dim.Render("←/→ or enter cycle   ↑/↓ move   q quit"))
	case rowModel:
		b.WriteString(m.styles.Dim.Render("enter pick model   ↑/↓ move   q quit"))
	case rowAuth:
		b.WriteString(m.styles.Dim.Render("enter edit auth   ↑/↓ move   q quit"))
	case rowKeybinding:
		b.WriteString(m.styles.Dim.Render("enter edit   ↑/↓ move   q quit"))
	case rowTheme:
		b.WriteString(m.styles.Dim.Render("enter pick theme   ↑/↓ move   q quit"))
	case rowAddProvider:
		b.WriteString(m.styles.Dim.Render("enter add provider   ↑/↓ move   q quit"))
	default:
		b.WriteString(m.styles.Dim.Render("↑/↓ move   enter select   q quit"))
	}

	return b.String()
}

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
			b.WriteString(m.styles.Normal.Render("  "+mod.ID) + "\n")
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
			b.WriteString(m.styles.Normal.Render("  "+a.label) + current + "\n")
		}
	}
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))
	return b.String()
}

func (m editorModel) renderThemePicker() string {
	var b strings.Builder
	palettes := theme.All()

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

func (m editorModel) renderEnterKeybinding() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")
	b.WriteString("Shell keybinding:\n\n")
	b.WriteString(m.kbInput.View() + "\n")
	b.WriteString("\n" + m.styles.Hint.Render("type the binding string, e.g. ^T, ^G, ^X"))
	b.WriteString("\n" + m.styles.Hint.Render("takes effect in new terminals, or run: eval \"$(termwise init zsh)\""))
	b.WriteString("\n\n" + m.styles.Dim.Render("enter confirm   esc cancel"))
	return b.String()
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
