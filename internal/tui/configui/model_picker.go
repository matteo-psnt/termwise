package configui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

type modelPickerModel struct {
	styles   configStyles
	spin     spinner.Model
	provider string
	pc       config.ProviderConfig
	current  string // pre-select this model ID after load
	models   []provider.Model
	cursor   int
	loading  bool
	err      string

	selected  string
	done      bool
	cancelled bool
}

func newModelPicker(provider string, pc config.ProviderConfig, current string, styles configStyles) modelPickerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return modelPickerModel{
		styles:   styles,
		spin:     sp,
		provider: provider,
		pc:       pc,
		current:  current,
		loading:  true,
	}
}

func (m modelPickerModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.fetchCmd())
}

func (m modelPickerModel) Update(msg tea.Msg) (modelPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case editorModelsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.models = msg.models
		m.cursor = 0
		for i, mod := range m.models {
			if mod.ID == m.current {
				m.cursor = i
				break
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "b":
			m.cancelled = true
			m.done = true
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.models)-1 {
				m.cursor++
			}
		case "enter", " ":
			if !m.loading && m.err == "" && len(m.models) > 0 {
				m.selected = m.models[m.cursor].ID
				m.done = true
			}
		}
	}
	return m, nil
}

func (m modelPickerModel) View() string {
	var b strings.Builder
	b.WriteString("Model for " + m.styles.Title.Render(m.provider) + ":\n\n")

	if m.loading && len(m.models) == 0 {
		b.WriteString(m.styles.Spinner.Render(m.spin.View()) + " Fetching models...")
		return b.String()
	}
	if m.err != "" {
		b.WriteString(m.styles.Error.Render("Error: "+m.err) + "\n\n")
		b.WriteString(m.styles.Dim.Render("esc back"))
		return b.String()
	}
	for i, mod := range m.models {
		if i == m.cursor {
			b.WriteString(m.styles.Selected.Render("▶ "+mod.ID) + "\n")
		} else {
			b.WriteString("  " + mod.ID + "\n")
		}
	}
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back"))
	return b.String()
}

func (m modelPickerModel) fetchCmd() tea.Cmd {
	provider := m.provider
	pc := m.pc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := config.ListProviderModels(ctx, provider, pc)
		if err != nil {
			return editorModelsMsg{providerName: provider, err: fmt.Errorf("listing models: %w", err)}
		}
		return editorModelsMsg{providerName: provider, models: models}
	}
}
