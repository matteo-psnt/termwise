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
