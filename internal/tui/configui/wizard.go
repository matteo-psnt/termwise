package configui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// ---------------------------------------------------------------------------
// Step enum
// ---------------------------------------------------------------------------

type wizardStep int

const (
	wizPickProvider wizardStep = iota
	wizPickAuth
	wizEnterValue
	wizWorking
	wizPickModel
	wizDone
	wizErr
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type wizardModel struct {
	styles  configStyles
	width   int
	height  int
	spin    spinner.Model
	exclude map[string]bool // providers to hide (already configured)

	step   wizardStep
	cursor int

	// selections
	provider   string // e.g. "anthropic"
	authMethod string // "env", "cmd", "keychain"

	// wizEnterValue input
	enterLabel    string
	enterHint     string
	enterFallback string // used as placeholder when input is empty
	input         textinput.Model

	// wizPickModel
	apiModels []provider.Model
	modelID   string

	// final result — set when saved
	Result *config.FileConfig

	// done is set when the wizard exits (quit or completed) so that an
	// embedding editor model can detect the exit without inspecting tea.Cmd.
	done bool

	// error display
	errMsg  string
	errBack wizardStep
}

func newWizardModel(hasDarkBg bool, themeName string) wizardModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.CharLimit = 256

	return wizardModel{
		styles: newStylesForTheme(hasDarkBg, themeName),
		spin:   sp,
		input:  ti,
		step:   wizPickProvider,
	}
}

// availableProviders returns the provider list with already-configured ones removed.
func (m wizardModel) availableProviders() []config.ProviderInfo {
	all := config.ProviderInfos()
	if len(m.exclude) == 0 {
		return all
	}
	out := make([]config.ProviderInfo, 0, len(all))
	for _, p := range all {
		if !m.exclude[p.Name] {
			out = append(out, p)
		}
	}
	return out
}

func (m wizardModel) selectedProvider() (config.ProviderInfo, bool) {
	providers := m.availableProviders()
	if len(providers) == 0 || m.cursor < 0 || m.cursor >= len(providers) {
		return config.ProviderInfo{}, false
	}
	return providers[m.cursor], true
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func (wizardModel) Init() tea.Cmd {
	return nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case wizModelsMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.errBack = wizEnterValue
			m.step = wizErr
			return m, nil
		}
		m.apiModels = msg.models
		m.cursor = 0
		m.step = wizPickModel
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward to textinput when in enter step.
	if m.step == wizEnterValue {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m wizardModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case wizPickProvider:
		return m.handlePickProviderKey(msg)
	case wizPickAuth:
		return m.handlePickAuthKey(msg)
	case wizEnterValue:
		return m.handleEnterValueKey(msg)
	case wizPickModel:
		return m.handlePickModelKey(msg)
	case wizDone:
		m.done = true
		return m, tea.Quit
	case wizErr:
		return m.handleErrKey(msg)
	}

	return m, nil
}

func (m wizardModel) handlePickProviderKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	providers := m.availableProviders()

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		m.done = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(providers)-1 {
			m.cursor++
		}
	case "enter", " ":
		selected, ok := m.selectedProvider()
		if !ok {
			return m, nil
		}

		m.provider = selected.Name
		m.cursor = 0
		if provider.HasNoAuth(m.provider) {
			m.authMethod = "env"
			m.setupEnterValue()
			m.step = wizEnterValue
			return m, nil
		}

		m.step = wizPickAuth
	}

	return m, nil
}

func (m wizardModel) handlePickAuthKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.done = true
		return m, tea.Quit
	case "b", "esc":
		m.cursor = 0
		m.step = wizPickProvider
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(authMethods)-1 {
			m.cursor++
		}
	case "enter", " ":
		m.authMethod = authMethods[m.cursor].id
		m.cursor = 0
		m.setupEnterValue()
		if m.authMethod == "env" && envVarDetected(m.provider) {
			m.input.Blur()
			m.step = wizWorking
			return m, tea.Batch(m.spin.Tick, m.fetchModelsCmd())
		}
		m.step = wizEnterValue
	}

	return m, nil
}

func (m wizardModel) handleEnterValueKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.done = true
		return m, tea.Quit
	case "esc":
		m.input.Blur()
		m.cursor = 0
		if provider.HasNoAuth(m.provider) {
			m.step = wizPickProvider
		} else {
			m.step = wizPickAuth
		}
	case "enter":
		if strings.TrimSpace(m.input.Value()) == "" && m.enterFallback == "" {
			// Don't submit; leave cursor in the field.
			return m, nil
		}
		m.input.Blur()
		m.step = wizWorking
		return m, tea.Batch(m.spin.Tick, m.fetchModelsCmd())
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m wizardModel) handlePickModelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.done = true
		return m, tea.Quit
	case "b", "esc":
		m.cursor = 0
		m.step = wizEnterValue
		m.input.Focus()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.apiModels)-1 {
			m.cursor++
		}
	case "enter", " ":
		m.modelID = m.apiModels[m.cursor].ID
		m.saveConfig()
		m.step = wizDone
	}

	return m, nil
}

func (m wizardModel) handleErrKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.done = true
		return m, tea.Quit
	case "b", "esc":
		m.cursor = 0
		m.step = m.errBack
		if m.step == wizEnterValue {
			m.input.Focus()
		}
	}

	return m, nil
}

// setupEnterValue configures the textinput for the current auth method.
func (m *wizardModel) setupEnterValue() {
	cfg := buildAuthInputConfig(m.provider, m.authMethod)
	m.enterLabel = cfg.Label
	m.enterHint = cfg.Hint
	m.enterFallback = cfg.Fallback
	m.input.Placeholder = cfg.Placeholder
	m.input.EchoMode = cfg.EchoMode
	m.input.SetValue("")
	m.input.Focus()
}

// fetchModelsCmd builds the ProviderConfig from wizard state and calls ListModels.
func (m wizardModel) fetchModelsCmd() tea.Cmd {
	provider := m.provider
	authMethod := m.authMethod
	inputVal := m.input.Value()
	fallback := m.enterFallback

	return func() tea.Msg {
		val := inputVal
		if val == "" {
			val = fallback
		}

		pc := buildProviderConfigFromAuthInput(provider, authMethod, val, fallback)
		if authMethod == "keychain" {
			// val is the raw API key — store it in the keychain.
			if err := config.StoreKeychain(provider, val); err != nil {
				return wizModelsMsg{err: fmt.Errorf("keychain write: %w", err)}
			}
			pc = config.ProviderConfig{
				Auth:          "keychain",
				KeychainEntry: config.DefaultKeychainEntry(provider),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		models, err := config.ListProviderModels(ctx, provider, pc)
		if err != nil {
			return wizModelsMsg{err: fmt.Errorf("listing models: %w", err)}
		}
		if len(models) == 0 {
			return wizModelsMsg{err: fmt.Errorf("no models returned by %s", provider)}
		}
		return wizModelsMsg{models: models}
	}
}

// saveConfig builds the final config and stores it on m.Result.
func (m *wizardModel) saveConfig() {
	val := m.input.Value()
	if val == "" {
		val = m.enterFallback
	}

	pc := buildProviderConfigFromAuthInput(m.provider, m.authMethod, val, m.enterFallback)
	pc.Model = m.modelID
	if m.authMethod == "keychain" {
		pc = config.ProviderConfig{
			Auth:          "keychain",
			KeychainEntry: config.DefaultKeychainEntry(m.provider),
			Model:         m.modelID,
		}
	}

	cfg := config.FileConfig{
		SelectedProvider: m.provider,
		Providers:        map[string]config.ProviderConfig{m.provider: pc},
	}
	m.Result = &cfg
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m wizardModel) View() tea.View {
	inner := m.renderInner()
	box := m.styles.Outer.Render(inner)
	content := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m wizardModel) renderInner() string {
	var b strings.Builder

	title := m.styles.Title.Render("termwise setup")
	b.WriteString(title + "\n\n")

	switch m.step {

	case wizPickProvider:
		providers := m.availableProviders()
		b.WriteString("Choose a provider:\n\n")
		if len(providers) == 0 {
			b.WriteString("All built-in providers are already configured.\n\n")
			b.WriteString(m.styles.Dim.Render("q quit"))
			break
		}
		for i, p := range providers {
			if i == m.cursor {
				b.WriteString(m.styles.Selected.Render("▶ "+p.DisplayName) + "\n")
			} else {
				b.WriteString(m.styles.Normal.Render("  "+p.DisplayName) + "\n")
			}
		}
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   q quit"))

	case wizPickAuth:
		b.WriteString("Auth method for " + m.styles.Title.Render(m.provider) + ":\n\n")
		b.WriteString(renderAuthMethodRows(m.provider, m.cursor, "", true, m.styles))
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))

	case wizEnterValue:
		b.WriteString(m.enterLabel + ":\n\n")
		b.WriteString(m.input.View() + "\n")
		if m.enterHint != "" {
			b.WriteString("\n" + m.styles.Hint.Render(m.enterHint))
		}
		b.WriteString("\n\n" + m.styles.Dim.Render("enter confirm   esc back"))

	case wizWorking:
		b.WriteString(m.styles.Spinner.Render(m.spin.View()) + " Connecting to " + m.provider + "...")

	case wizPickModel:
		b.WriteString("Choose a model:\n\n")
		for i, mod := range m.apiModels {
			if i == m.cursor {
				b.WriteString(m.styles.Selected.Render("▶ "+mod.ID) + "\n")
			} else {
				b.WriteString(m.styles.Normal.Render("  "+mod.ID) + "\n")
			}
		}
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))

	case wizDone:
		b.WriteString(m.styles.Success.Render("✓ Done!") + "\n\n")
		b.WriteString("termwise is configured with " + m.styles.Title.Render(m.provider) + ".\n")
		b.WriteString("Model: " + m.styles.Dim.Render(m.modelID) + "\n\n")
		b.WriteString(m.styles.Dim.Render("press any key to continue"))

	case wizErr:
		b.WriteString(m.styles.Error.Render("Error") + "\n\n")
		b.WriteString(m.errMsg + "\n\n")
		b.WriteString(m.styles.Dim.Render("esc/b go back   q quit"))
	}

	return b.String()
}
