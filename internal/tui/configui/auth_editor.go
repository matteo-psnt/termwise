package configui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/config"
)

type authStep int

const (
	authStepPickMethod authStep = iota
	authStepEnterValue
	authStepVerify
)

type authEditorModel struct {
	styles   configStyles
	spin     spinner.Model
	provider string
	existing config.ProviderConfig
	isActive bool // whether this is the active provider
	connOK   bool // whether the provider is already verified

	step         authStep
	methodCursor int
	method       string

	// value entry
	input    textinput.Model
	label    string
	hint     string
	fallback string
	inputErr string

	// result
	result    config.ProviderConfig
	done      bool
	cancelled bool
}

func newAuthEditor(provider string, pc config.ProviderConfig, isActive bool, connOK bool, styles configStyles) authEditorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.CharLimit = 256

	cur := pc.Auth
	if cur == "" {
		cur = "env"
	}
	methodCursor := 0
	for i, am := range authMethods {
		if am.id == cur {
			methodCursor = i
			break
		}
	}

	m := authEditorModel{
		styles:       styles,
		spin:         sp,
		input:        ti,
		provider:     provider,
		existing:     pc,
		isActive:     isActive,
		connOK:       connOK,
		methodCursor: methodCursor,
	}

	// Ollama skips the method picker — go straight to URL entry.
	if provider == "ollama" {
		m.method = "env"
		m.setupInput()
		m.step = authStepEnterValue
	} else {
		m.step = authStepPickMethod
	}
	return m
}

func (m authEditorModel) Init() tea.Cmd {
	if m.step == authStepEnterValue {
		return textinput.Blink
	}
	return nil
}

func (m authEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case editorAuthMsg:
		if m.step != authStepVerify {
			return m, nil
		}
		if !msg.ok {
			m.inputErr = msg.err.Error()
			m.input.Focus()
			m.step = authStepEnterValue
			return m, textinput.Blink
		}
		m.result = m.buildResult()
		m.done = true
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case authStepPickMethod:
			return m.handlePickMethod(msg)
		case authStepEnterValue:
			return m.handleEnterValue(msg)
		}
	}

	if m.step == authStepEnterValue {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m authEditorModel) handlePickMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		m.cancelled = true
		m.done = true
	case "up", "k":
		if m.methodCursor > 0 {
			m.methodCursor--
		}
	case "down", "j":
		if m.methodCursor < len(authMethods)-1 {
			m.methodCursor++
		}
	case "enter", " ":
		selected := authMethods[m.methodCursor].id
		existingAuth := m.existing.Auth
		if existingAuth == "" {
			existingAuth = "env"
		}
		// Already on this method and provider is connected — nothing to do.
		if selected == existingAuth && m.isActive && m.connOK {
			m.cancelled = true
			m.done = true
			return m, nil
		}
		m.method = selected
		m.setupInput()
		// Env var already detected — skip entry, go straight to verify.
		if selected == "env" && envVarDetected(m.provider) {
			m.input.Blur()
			m.step = authStepVerify
			return m, tea.Batch(m.spin.Tick, m.verifyCmd())
		}
		// Same non-keychain method: skip entry, re-verify immediately.
		if selected == existingAuth && selected != "keychain" {
			m.input.Blur()
			m.step = authStepVerify
			return m, tea.Batch(m.spin.Tick, m.verifyCmd())
		}
		m.step = authStepEnterValue
		return m, textinput.Blink
	}
	return m, nil
}

func (m authEditorModel) handleEnterValue(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input.Blur()
		m.inputErr = ""
		if m.provider == "ollama" {
			m.cancelled = true
			m.done = true
		} else {
			m.step = authStepPickMethod
		}
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" && m.fallback == "" {
			m.inputErr = "value cannot be empty"
			return m, nil
		}
		m.input.Blur()
		m.inputErr = ""
		m.step = authStepVerify
		return m, tea.Batch(m.spin.Tick, m.verifyCmd())
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// setupInput populates the textinput from buildAuthInputConfig and restores
// any existing configured value.
func (m *authEditorModel) setupInput() {
	cfg := buildAuthInputConfig(m.provider, m.method)
	m.label = cfg.Label
	m.hint = cfg.Hint
	m.fallback = cfg.Fallback
	m.input.Placeholder = cfg.Placeholder
	m.input.EchoMode = cfg.EchoMode

	switch {
	case m.provider == "ollama":
		m.input.SetValue(m.existing.BaseURL)
	case m.method == "env":
		m.input.SetValue(m.existing.EnvVar)
	case m.method == "cmd":
		m.input.SetValue(m.existing.APIKeyCmd)
	default:
		m.input.SetValue("")
	}
	m.inputErr = ""
	m.input.Focus()
}

// buildResult constructs the ProviderConfig from the current input state.
func (m authEditorModel) buildResult() config.ProviderConfig {
	if m.method == "keychain" {
		return config.ProviderConfig{
			Auth:          "keychain",
			KeychainEntry: config.DefaultKeychainEntry(m.provider),
		}
	}
	return buildProviderConfigFromAuthInput(m.provider, m.method, m.input.Value(), m.fallback)
}

func (m authEditorModel) verifyCmd() tea.Cmd {
	provider := m.provider
	method := m.method
	inputVal := m.input.Value()
	fallback := m.fallback

	return func() tea.Msg {
		var pc config.ProviderConfig
		if method == "keychain" {
			if err := config.StoreKeychain(provider, inputVal); err != nil {
				return editorAuthMsg{providerName: provider, ok: false, err: fmt.Errorf("keychain write: %w", err)}
			}
			pc = config.ProviderConfig{
				Auth:          "keychain",
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

func (m authEditorModel) View() string {
	switch m.step {
	case authStepPickMethod:
		return m.viewPickMethod()
	case authStepEnterValue:
		return m.viewEnterValue()
	case authStepVerify:
		return m.viewVerify()
	}
	return ""
}

func (m authEditorModel) viewPickMethod() string {
	var b strings.Builder
	b.WriteString("Auth method for " + m.styles.Title.Render(m.provider) + ":\n\n")
	cur := m.existing.Auth
	if cur == "" {
		cur = "env"
	}
	b.WriteString(renderAuthMethodRows(m.provider, m.methodCursor, cur, false, m.styles))
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back"))
	return b.String()
}

func (m authEditorModel) viewEnterValue() string {
	var b strings.Builder
	b.WriteString(m.label + ":\n\n")
	b.WriteString(m.input.View() + "\n")
	if m.hint != "" {
		b.WriteString("\n" + m.styles.Hint.Render(m.hint))
	}
	if m.inputErr != "" {
		b.WriteString("\n\n" + m.styles.Error.Render("Error: "+m.inputErr))
	}
	b.WriteString("\n\n" + m.styles.Dim.Render("enter verify & save   esc back"))
	return b.String()
}

func (m authEditorModel) viewVerify() string {
	return m.styles.Spinner.Render(m.spin.View()) + " Verifying with " + m.provider + "..."
}
