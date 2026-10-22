package configui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
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

	// keychain records keys written, so the editor that embeds the wizard
	// can undo them on revert. Nil for the first-run wizard.
	keychain *keychainJournal

	step   wizardStep
	cursor int

	// selections
	provider   string // e.g. "anthropic"
	authMethod string // "env", "cmd", "keychain"

	// wizEnterValue input
	input    authInput
	inputErr string

	// wizPickModel
	apiModels []provider.Model
	modelID   string

	// final result — set when saved
	result *config.FileConfig

	// done is set when the wizard exits (quit or completed) so that an
	// embedding editor model can detect the exit without inspecting tea.Cmd.
	done bool

	// error display
	errMsg  string
	errBack wizardStep
}

// newWizardModel starts the wizard at the provider list, hiding the providers
// in exclude and with the cursor on the first one whose key is already set.
func newWizardModel(hasDarkBg bool, themeName string, exclude map[string]bool) wizardModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := wizardModel{
		styles:  newStylesForTheme(hasDarkBg, themeName),
		spin:    sp,
		exclude: exclude,
		input:   newAuthInput(),
		step:    wizPickProvider,
	}
	return m.preselectDetected()
}

// availableProviders returns the provider list with already-configured ones removed.
func (m wizardModel) availableProviders() []provider.Info {
	all := provider.Catalog()
	if len(m.exclude) == 0 {
		return all
	}
	out := make([]provider.Info, 0, len(all))
	for _, p := range all {
		if !m.exclude[p.Name] {
			out = append(out, p)
		}
	}
	return out
}

// preselectDetected moves the cursor to the first provider whose API key is
// already in the environment, so the common case is a single enter press.
func (m wizardModel) preselectDetected() wizardModel {
	for i, p := range m.availableProviders() {
		if envVarDetected(p.Name) {
			m.cursor = i
			break
		}
	}
	return m
}

func (m wizardModel) selectedProvider() (provider.Info, bool) {
	providers := m.availableProviders()
	if len(providers) == 0 || m.cursor < 0 || m.cursor >= len(providers) {
		return provider.Info{}, false
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
		m.cursor = defaultModelIndex(m.provider, msg.models)
		m.step = wizPickModel
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward to textinput when in enter step.
	if m.step == wizEnterValue {
		var cmd tea.Cmd
		m.input.Model, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m wizardModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// ctrl+c quits from every step, including wizWorking, which takes no
	// other keys while the fetch (up to 15s) is in flight.
	if msg.String() == "ctrl+c" {
		m.done = true
		return m, tea.Quit
	}

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

	if c, ok := moveListCursor(msg.String(), m.cursor, len(providers)); ok {
		m.cursor = c
		return m, nil
	}
	switch msg.String() {
	case "q", "esc":
		m.done = true
		return m, tea.Quit
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
	if c, ok := moveListCursor(msg.String(), m.cursor, len(authMethods)); ok {
		m.cursor = c
		return m, nil
	}
	switch msg.String() {
	case "q":
		m.done = true
		return m, tea.Quit
	case "b", "esc":
		m.cursor = 0
		m.step = wizPickProvider
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
	case "esc":
		m.input.Blur()
		m.inputErr = ""
		m.cursor = 0
		if provider.HasNoAuth(m.provider) {
			m.step = wizPickProvider
		} else {
			m.step = wizPickAuth
		}
	case "enter":
		if m.input.missing() {
			// Don't submit; leave cursor in the field.
			m.inputErr = errEmptyValue
			return m, nil
		}
		m.input.Blur()
		m.inputErr = ""
		m.step = wizWorking
		return m, tea.Batch(m.spin.Tick, m.fetchModelsCmd())
	default:
		var cmd tea.Cmd
		m.input.Model, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m wizardModel) handlePickModelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if c, ok := moveListCursor(msg.String(), m.cursor, len(m.apiModels)); ok {
		m.cursor = c
		return m, nil
	}
	switch msg.String() {
	case "q":
		m.done = true
		return m, tea.Quit
	case "b", "esc":
		m.cursor = 0
		m.step = wizEnterValue
		m.input.Focus()
	case "enter", " ":
		m.modelID = m.apiModels[m.cursor].ID
		m.saveConfig()
		m.step = wizDone
	}

	return m, nil
}

func (m wizardModel) handleErrKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
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

// setupEnterValue sets the input up, empty, for the current auth method.
func (m *wizardModel) setupEnterValue() {
	m.input.setup(m.provider, m.authMethod, "")
	m.inputErr = ""
}

// fetchModelsCmd builds the ProviderConfig from wizard state and calls ListModels.
func (m wizardModel) fetchModelsCmd() tea.Cmd {
	provider := m.provider
	inputVal := m.input.Value()
	pc := buildProviderConfigFromAuthInput(provider, m.authMethod, inputVal, m.input.fallback)
	journal := m.keychain

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		models, err := listModelsFor(ctx, provider, pc, inputVal, journal)
		if err != nil {
			return wizModelsMsg{err: fmt.Errorf("listing models: %w", err)}
		}
		return wizModelsMsg{models: models}
	}
}

// saveConfig builds the final config and stores it on m.result.
func (m *wizardModel) saveConfig() {
	pc := buildProviderConfigFromAuthInput(m.provider, m.authMethod, m.input.Value(), m.input.fallback)
	pc.Model = m.modelID

	cfg := config.FileConfig{
		SelectedProvider: m.provider,
		Providers:        map[string]config.ProviderConfig{m.provider: pc},
	}
	m.result = &cfg
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
		detected := false
		for i, p := range providers {
			badge := ""
			if envVarDetected(p.Name) {
				badge = "  " + m.styles.Success.Render("●")
				detected = true
			}
			b.WriteString(renderListRow(m.styles, i == m.cursor, p.DisplayName, badge) + "\n")
		}
		if detected {
			b.WriteString("\n" + m.styles.Hint.Render("● API key found in your environment"))
		}
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   q quit"))

	case wizPickAuth:
		b.WriteString("Auth method for " + m.styles.Title.Render(m.provider) + ":\n\n")
		b.WriteString(renderAuthMethodRows(m.provider, m.cursor, "", true, m.styles))
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back   q quit"))

	case wizEnterValue:
		b.WriteString(m.input.view(m.styles, m.inputErr, "enter confirm   esc back"))

	case wizWorking:
		b.WriteString(m.styles.Spinner.Render(m.spin.View()) + " Connecting to " + m.provider + "...")

	case wizPickModel:
		b.WriteString("Choose a model:\n\n")
		b.WriteString(renderModelList(m.styles, m.apiModels, m.cursor, m.height, models.DefaultModel(m.provider)))
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
