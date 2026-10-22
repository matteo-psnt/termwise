package configui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	providerpkg "github.com/matteo-psnt/termwise/internal/provider"
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
	input    authInput
	inputErr string

	// result
	result    config.ProviderConfig
	done      bool
	cancelled bool
}

func newAuthEditor(provider string, pc config.ProviderConfig, isActive bool, connOK bool, styles configStyles) authEditorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	cur := pc.AuthMethod()
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
		input:        newAuthInput(),
		provider:     provider,
		existing:     pc,
		isActive:     isActive,
		connOK:       connOK,
		methodCursor: methodCursor,
	}

	// Providers without auth credentials skip the method picker and go
	// straight to URL entry.
	if providerpkg.HasNoAuth(provider) {
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

func (m authEditorModel) Update(msg tea.Msg) (authEditorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case editorAuthMsg:
		if m.step != authStepVerify {
			return m, nil
		}
		if msg.err != nil {
			m.inputErr = msg.err.Error()
			m.input.Focus()
			m.step = authStepEnterValue
			return m, textinput.Blink
		}
		m.result = m.buildResult()
		m.done = true
		return m, nil

	case tea.KeyPressMsg:
		switch m.step {
		case authStepPickMethod:
			return m.handlePickMethod(msg)
		case authStepEnterValue:
			return m.handleEnterValue(msg)
		}
	}

	if m.step == authStepEnterValue {
		var cmd tea.Cmd
		m.input.Model, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m authEditorModel) handlePickMethod(msg tea.KeyPressMsg) (authEditorModel, tea.Cmd) {
	if c, ok := moveListCursor(msg.String(), m.methodCursor, len(authMethods)); ok {
		m.methodCursor = c
		return m, nil
	}
	switch msg.String() {
	case "esc", "b":
		m.cancelled = true
		m.done = true
	case "enter", " ":
		selected := authMethods[m.methodCursor].id
		existingAuth := m.existing.AuthMethod()
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

func (m authEditorModel) handleEnterValue(msg tea.KeyPressMsg) (authEditorModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input.Blur()
		m.inputErr = ""
		if providerpkg.HasNoAuth(m.provider) {
			m.cancelled = true
			m.done = true
		} else {
			m.step = authStepPickMethod
		}
	case "enter":
		if m.input.missing() {
			m.inputErr = "value cannot be empty"
			return m, nil
		}
		m.input.Blur()
		m.inputErr = ""
		m.step = authStepVerify
		return m, tea.Batch(m.spin.Tick, m.verifyCmd())
	default:
		var cmd tea.Cmd
		m.input.Model, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// setupInput sets the input up for m.method, restoring any existing
// configured value. A stored API key is never read back.
func (m *authEditorModel) setupInput() {
	var initial string
	switch {
	case providerpkg.HasNoAuth(m.provider):
		initial = m.existing.BaseURL
	case m.method == "env":
		initial = m.existing.EnvVar
	case m.method == "cmd":
		initial = m.existing.APIKeyCmd
	}
	m.input.setup(m.provider, m.method, initial)
	m.inputErr = ""
}

// buildResult constructs the ProviderConfig from the current input state.
// Only auth is being edited, so the rest of the existing block is kept; a
// keyed provider's base URL is a proxy or gateway the key belongs to.
func (m authEditorModel) buildResult() config.ProviderConfig {
	pc := buildProviderConfigFromAuthInput(m.provider, m.method, m.input.Value(), m.input.fallback)
	pc.Model = m.existing.Model
	pc.Effort = m.existing.Effort
	if !providerpkg.HasNoAuth(m.provider) {
		pc.BaseURL = m.existing.BaseURL
	}
	return pc
}

func (m authEditorModel) verifyCmd() tea.Cmd {
	provider := m.provider
	inputVal := m.input.Value()
	pc := m.buildResult()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := listModelsFor(ctx, provider, pc, inputVal)
		return editorAuthMsg{err: err}
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
	cur := m.existing.AuthMethod()
	b.WriteString(renderAuthMethodRows(m.provider, m.methodCursor, cur, false, m.styles))
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back"))
	return b.String()
}

func (m authEditorModel) viewEnterValue() string {
	return m.input.view(m.styles, m.inputErr, "enter verify & save   esc back")
}

func (m authEditorModel) viewVerify() string {
	return m.styles.Spinner.Render(m.spin.View()) + " Verifying with " + m.provider + "..."
}
