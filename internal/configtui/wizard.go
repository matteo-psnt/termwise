package configtui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/config"

	// Register providers so GetProvider works.
	_ "github.com/matteo-psnt/termwise/internal/ai/anthropic"
	_ "github.com/matteo-psnt/termwise/internal/ai/openaicompat"
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
// Data tables
// ---------------------------------------------------------------------------

var authMethods = []struct {
	id    string
	label string
}{
	{"env", "Environment variable"},
	{"keychain", "macOS Keychain"},
	{"cmd", "Shell command"},
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type wizardModel struct {
	styles configStyles
	width  int
	height int
	spin   spinner.Model

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
	apiModels []ai.Model
	modelID   string

	// final result — set when saved
	Result *config.Config

	// done is set when the wizard exits (quit or completed) so that an
	// embedding editor model can detect the exit without inspecting tea.Cmd.
	done bool

	// error display
	errMsg  string
	errBack wizardStep
}

func newWizardModel(r *lipgloss.Renderer, themeName string) wizardModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.CharLimit = 256

	return wizardModel{
		styles: newStylesForTheme(r, themeName),
		spin:   sp,
		input:  ti,
		step:   wizPickProvider,
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func (m wizardModel) Init() tea.Cmd {
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

	case tea.KeyMsg:
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

func (m wizardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {

	case wizPickProvider:
		providers := config.ProviderInfos()
		switch msg.String() {
		case "q", "ctrl+c":
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
			m.provider = providers[m.cursor].Name
			m.cursor = 0
			if m.provider == "ollama" {
				// No API key needed — ask for base URL instead.
				m.authMethod = "env"
				m.enterLabel = "Ollama base URL"
				m.enterHint = "leave blank for default (" + defaultOllamaBaseURL + ")"
				m.enterFallback = defaultOllamaBaseURL
				m.input.Placeholder = defaultOllamaBaseURL
				m.input.SetValue("")
				m.input.Focus()
				m.step = wizEnterValue
			} else {
				m.step = wizPickAuth
			}
		}

	case wizPickAuth:
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
			m.step = wizEnterValue
		}

	case wizEnterValue:
		switch msg.String() {
		case "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "esc":
			m.input.Blur()
			m.cursor = 0
			if m.provider == "ollama" {
				m.step = wizPickProvider
			} else {
				m.step = wizPickAuth
			}
		case "enter":
			if strings.TrimSpace(m.input.Value()) == "" && m.enterFallback == "" {
				// Don't submit — leave cursor in the field.
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

	case wizPickModel:
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
			if err := m.saveConfig(); err != nil {
				m.errMsg = err.Error()
				m.errBack = wizPickModel
				m.step = wizErr
			} else {
				m.step = wizDone
			}
		}

	case wizDone:
		m.done = true
		return m, tea.Quit

	case wizErr:
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
	}

	return m, nil
}

// setupEnterValue configures the textinput for the current auth method.
func (m *wizardModel) setupEnterValue() {
	m.input.EchoMode = textinput.EchoNormal

	switch m.authMethod {
	case "env":
		defVar := config.DefaultEnvVar(m.provider)
		m.enterLabel = "Environment variable name"
		m.enterHint = "the env var that holds your API key"
		m.enterFallback = defVar
		m.input.Placeholder = defVar
	case "cmd":
		m.enterLabel = "Shell command"
		m.enterHint = "command whose stdout is the API key"
		m.enterFallback = ""
		m.input.Placeholder = "op read op://vault/item/field"
	case "keychain":
		m.enterLabel = "API key"
		m.enterHint = "will be stored securely in macOS Keychain"
		m.enterFallback = ""
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "sk-..."
	}
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
				AuthMethod:    "keychain",
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

// saveConfig builds and saves the final config, also setting m.Result.
func (m *wizardModel) saveConfig() error {
	val := m.input.Value()
	if val == "" {
		val = m.enterFallback
	}

	pc := buildProviderConfigFromAuthInput(m.provider, m.authMethod, val, m.enterFallback)
	pc.Model = m.modelID
	if m.authMethod == "keychain" {
		pc = config.ProviderConfig{
			AuthMethod:    "keychain",
			KeychainEntry: config.DefaultKeychainEntry(m.provider),
			Model:         m.modelID,
		}
	}

	cfg := config.Config{
		ActiveProvider: m.provider,
		Providers:      map[string]config.ProviderConfig{m.provider: pc},
	}
	m.Result = &cfg
	return nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m wizardModel) View() string {
	inner := m.renderInner()
	box := m.styles.Outer.Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m wizardModel) renderInner() string {
	var b strings.Builder

	title := m.styles.Title.Render("termwise setup")
	b.WriteString(title + "\n\n")

	switch m.step {

	case wizPickProvider:
		providers := config.ProviderInfos()
		b.WriteString("Choose a provider:\n\n")
		for i, p := range providers {
			if i == m.cursor {
				b.WriteString(m.styles.Selected.Render("▶ "+p.Label) + "\n")
			} else {
				b.WriteString(m.styles.Normal.Render("  "+p.Label) + "\n")
			}
		}
		b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   q quit"))

	case wizPickAuth:
		b.WriteString("Auth method for " + m.styles.Title.Render(m.provider) + ":\n\n")
		for i, a := range authMethods {
			if i == m.cursor {
				b.WriteString(m.styles.Selected.Render("▶ "+a.label) + "\n")
			} else {
				b.WriteString(m.styles.Normal.Render("  "+a.label) + "\n")
			}
		}
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
