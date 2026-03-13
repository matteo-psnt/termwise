package configtui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/config"

	_ "github.com/matteo-psnt/termwise/internal/ai/anthropic"
	_ "github.com/matteo-psnt/termwise/internal/ai/openaicompat"
)

// ---------------------------------------------------------------------------
// Editor state
// ---------------------------------------------------------------------------

type editorState int

const (
	editorNormal    editorState = iota
	editorPickModel             // inline model picker open
)

// editorRow is one navigable row in the editor.
type editorRow struct {
	kind     editorRowKind
	provider string // for rowActiveProvider and rowModel
}

type editorRowKind int

const (
	rowActiveProvider editorRowKind = iota
	rowModel
	rowSave
	rowCancel
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type editorModel struct {
	styles configStyles
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

	// Exit
	saved bool
	quit  bool
	err   error
}

func newEditorModel(cfgPath string, cfg config.Config, r *lipgloss.Renderer) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := editorModel{
		styles:  newStyles(r),
		spin:    sp,
		cfgPath: cfgPath,
		cfg:     cfg,
	}
	m.buildRows()
	return m
}

// buildRows constructs the flat list of navigable rows from cfg.
func (m *editorModel) buildRows() {
	m.rows = []editorRow{
		{kind: rowActiveProvider},
	}
	// Add one model row per configured provider (sorted).
	providers := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	for _, name := range providers {
		m.rows = append(m.rows, editorRow{kind: rowModel, provider: name})
	}
	m.rows = append(m.rows, editorRow{kind: rowSave}, editorRow{kind: rowCancel})
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
		// Scroll to current model if present.
		current := m.cfg.Providers[m.pickingProvider].Model
		for i, mod := range m.modelList {
			if mod.ID == current {
				m.modelCursor = i
				break
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m editorModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state == editorPickModel {
		return m.handleModelPickerKey(msg)
	}

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
			// Cycle through configured providers.
			providers := m.sortedProviders()
			if len(providers) == 0 {
				return m, nil
			}
			idx := indexOf(providers, m.cfg.ActiveProvider)
			m.cfg.ActiveProvider = providers[(idx+1)%len(providers)]
			// Recheck connectivity for new active provider.
			m.connChecked = false
			m.connOK = false
			m.connErr = ""
			return m, tea.Batch(m.spin.Tick, m.checkConnectivityCmd())

		case rowModel:
			return m.openModelPicker(row.provider)

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
		// Cycle active provider backwards.
		row := m.rows[m.cursor]
		if row.kind == rowActiveProvider {
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
		row := m.rows[m.cursor]
		if row.kind == rowActiveProvider {
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

// openModelPicker starts the model picker sub-state for the given provider.
func (m editorModel) openModelPicker(providerName string) (tea.Model, tea.Cmd) {
	m.state = editorPickModel
	m.pickingProvider = providerName
	m.modelList = nil
	m.modelCursor = 0
	m.modelLoading = true
	m.modelErr = ""
	return m, tea.Batch(m.spin.Tick, m.fetchModelsCmd(providerName))
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

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m editorModel) View() string {
	inner := m.renderInner()
	box := m.styles.Outer.Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m editorModel) renderInner() string {
	if m.state == editorPickModel {
		return m.renderModelPicker()
	}
	return m.renderNormal()
}

func (m editorModel) renderNormal() string {
	var b strings.Builder

	b.WriteString(m.styles.Title.Render("termwise config") + "\n\n")

	for i, row := range m.rows {
		focused := i == m.cursor
		prefix := "  "
		if focused {
			prefix = m.styles.Selected.Render("▶ ")
		} else {
			prefix = "  "
		}

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
			label := row.provider + " model: "
			val := pc.Model
			active := ""
			if row.provider == m.cfg.ActiveProvider {
				active = m.styles.Dim.Render(" (active)")
			}
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(label+val) + active + "\n")
			} else {
				b.WriteString(prefix + label + m.styles.Normal.Render(val) + active + "\n")
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

	// Context-sensitive hint.
	row := m.rows[m.cursor]
	switch row.kind {
	case rowActiveProvider:
		b.WriteString(m.styles.Dim.Render("←/→ or enter cycle   ↑/↓ move   q quit"))
	case rowModel:
		b.WriteString(m.styles.Dim.Render("enter pick model   ↑/↓ move   q quit"))
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

func (m editorModel) connIndicator() string {
	if !m.connChecked {
		return m.styles.Spinner.Render(m.spin.View())
	}
	if m.connOK {
		return m.styles.Success.Render("●")
	}
	return m.styles.Error.Render("●")
}
