package configui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
)

type editorModel struct {
	styles    configStyles
	hasDarkBg bool
	width     int
	height    int
	spin      spinner.Model

	cfgPath string
	cfg     config.FileConfig
	initial config.FileConfig // snapshot taken on open; restored on esc

	rows   []editorRow
	cursor int

	// Connectivity check for the active provider.
	connChecked bool
	connOK      bool

	// Sub-models — at most one non-nil at a time.
	modelPicker *modelPickerModel
	authEditor  *authEditorModel
	keybinding  *keybindingModel
	themePicker *themePickerModel
	enumPicker  *enumPickerModel
	addWizard   *wizardModel

	// commit stores the value picked in the open keybinding, theme or enum
	// sub-model. It is set when the sub-model opens, so the update handlers
	// don't need to know whether they are editing a setting or a provider's
	// effort.
	commit func(cfg *config.FileConfig, val string)

	err error
}

func newEditorModel(cfgPath string, cfg config.FileConfig, hasDarkBg bool) editorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	m := editorModel{
		styles:    newStylesForTheme(hasDarkBg, cfg.Settings.Theme),
		hasDarkBg: hasDarkBg,
		spin:      sp,
		cfgPath:   cfgPath,
		cfg:       cfg,
		initial:   cfg.Clone(),
	}
	m.buildRows()
	return m
}

func (m editorModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.checkConnectivityCmd())
}
