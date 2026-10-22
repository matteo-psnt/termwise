package configui

import "github.com/matteo-psnt/termwise/internal/provider"

// wizModelsMsg is returned when the background validate+fetch completes in the wizard.
type wizModelsMsg struct {
	models []provider.Model
	err    error
}

// editorModelsMsg is returned when a background model fetch completes in the editor.
//
// It is deliberately a separate type from wizModelsMsg: a model picker that
// was cancelled mid-fetch still delivers its result, and if the add-provider
// wizard is open by then the distinct type is what keeps the wizard from
// taking the stale list as its own.
type editorModelsMsg struct {
	models []provider.Model
	err    error
}

// editorConnMsg is returned when the background connectivity check completes.
// A nil err means the provider is reachable.
type editorConnMsg struct {
	err error
}

// editorAuthMsg is returned when background auth verification completes in
// the editor. A nil err means the credentials work.
type editorAuthMsg struct {
	err error
}
