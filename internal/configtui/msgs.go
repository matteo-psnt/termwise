package configtui

import "github.com/matteo-psnt/termwise/internal/ai"

// wizModelsMsg is returned when the background validate+fetch completes in the wizard.
type wizModelsMsg struct {
	models []ai.Model
	err    error
}

// editorModelsMsg is returned when a background model fetch completes in the editor.
type editorModelsMsg struct {
	providerName string
	models       []ai.Model
	err          error
}

// editorConnMsg is returned when the background connectivity check completes.
type editorConnMsg struct {
	ok  bool
	err error
}
