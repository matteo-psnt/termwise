package configui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
)

type modelPickerModel struct {
	styles   configStyles
	spin     spinner.Model
	height   int // screen height, to size the scrolling list
	provider string
	seq      int // tags this picker's fetch; see editorModelsMsg
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

func newModelPicker(provider string, pc config.ProviderConfig, current string, seq, height int, styles configStyles) modelPickerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return modelPickerModel{
		styles:   styles,
		spin:     sp,
		height:   height,
		provider: provider,
		seq:      seq,
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
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case editorModelsMsg:
		if msg.provider != m.provider || msg.seq != m.seq {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.models = msg.models
		m.cursor = modelIndex(m.models, m.current)
		return m, nil

	case tea.KeyPressMsg:
		if c, ok := moveListCursor(msg.String(), m.cursor, len(m.models)); ok {
			m.cursor = c
			return m, nil
		}
		switch msg.String() {
		case "esc", "b":
			m.cancelled = true
			m.done = true
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
	b.WriteString(renderModelList(m.styles, m.models, m.cursor, m.height, ""))
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ move   enter select   esc back"))
	return b.String()
}

func (m modelPickerModel) fetchCmd() tea.Cmd {
	provider := m.provider
	seq := m.seq
	pc := m.pc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		list, err := config.ListProviderModels(ctx, provider, pc)
		if err != nil {
			return editorModelsMsg{provider: provider, seq: seq, err: fmt.Errorf("listing models: %w", err)}
		}
		return editorModelsMsg{provider: provider, seq: seq, models: list}
	}
}

// renderModelList renders the rows of a model list that fit on a screen
// height rows tall, scrolled to keep cursor visible, with markers for what is
// cut off. The model whose ID is recommended gets a badge.
func renderModelList(styles configStyles, list []provider.Model, cursor, height int, recommended string) string {
	var b strings.Builder
	start, end := listWindow(len(list), cursor, modelRows(height))
	if start > 0 {
		b.WriteString(styles.Dim.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for i := start; i < end; i++ {
		id := list[i].ID
		badge := ""
		if id == recommended {
			badge = styles.Dim.Render("  (recommended)")
		}
		b.WriteString(renderListRow(styles, i == cursor, id, badge) + "\n")
	}
	if end < len(list) {
		b.WriteString(styles.Dim.Render(fmt.Sprintf("  ↓ %d more", len(list)-end)) + "\n")
	}
	return b.String()
}

// defaultModelIndex returns the position of the provider's recommended model
// in list, or 0 when the provider did not return it.
func defaultModelIndex(providerName string, list []provider.Model) int {
	return modelIndex(list, models.DefaultModel(providerName))
}

// modelIndex returns the position of the model with the given ID in list, or
// 0 when it isn't there.
func modelIndex(list []provider.Model, id string) int {
	for i, mod := range list {
		if mod.ID == id {
			return i
		}
	}
	return 0
}

// modelRows is how many model rows fit in a box on a screen height rows tall.
// The rest of the screen is title, prompt, scroll markers, help line and the
// box's own border; the wizard and the editor's picker lay these out alike.
func modelRows(height int) int {
	const chrome = 14
	if height <= 0 {
		return 15
	}
	return max(height-chrome, 5)
}
