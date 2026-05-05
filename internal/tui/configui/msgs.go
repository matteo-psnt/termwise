package configui

import "github.com/matteo-psnt/termwise/internal/provider"

// wizModelsMsg is returned when the background validate+fetch completes in the wizard.
type wizModelsMsg struct {
	models []provider.Model
	err    error
}

// editorModelsMsg is returned when a background model fetch completes in the editor.
type editorModelsMsg struct {
	providerName string
	models       []provider.Model
	err          error
}

// editorConnMsg is returned when the background connectivity check completes.
type editorConnMsg struct {
	ok  bool
	err error
}

// editorAuthMsg is returned when background auth verification completes in the editor.
type editorAuthMsg struct {
	providerName string
	ok           bool
	err          error
}
