package configui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/config"
)

type editorModel struct {
	styles configStyles
	r      *lipgloss.Renderer
	width  int
	height int
	spin   spinner.Model

	cfgPath string
	cfg     config.FileConfig
	initial config.FileConfig // snapshot taken on open; restored on esc

	rows   []editorRow
	cursor int

	// Connectivity check for the active provider.
	connChecked bool
	connOK      bool
	connErr     string

	// Sub-models — at most one non-nil at a time.
	modelPicker *modelPickerModel
	authEditor  *authEditorModel
	keybinding  *keybindingModel
	themePicker *themePickerModel
	enumPicker  *enumPickerModel
	addWizard   *wizardModel

	// Index into config.Settings for the currently active sub-model.
	// Mutually exclusive with activeEffortProvider.
	activeSettingIdx int

	// When the enum picker is editing a provider's Effort, this names the
	// provider. Empty otherwise. Mutually exclusive with activeSettingIdx use.
	activeEffortProvider string

	err error
}

func newEditorModel(cfgPath string, cfg config.FileConfig, r *lipgloss.Renderer) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	m := editorModel{
		styles:  newStylesForTheme(r, cfg.Settings.Theme),
		r:       r,
		spin:    sp,
		cfgPath: cfgPath,
		cfg:     cfg,
		initial: cfg.Clone(),
	}
	m.buildRows()
	return m
}

func (m editorModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
}
