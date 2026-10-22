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
	// provider and seq identify the picker that asked, so a newer picker
	// doesn't take the late result of one that was cancelled.
	provider string
	seq      int
	models   []provider.Model
	err      error
}

// editorConnMsg is returned when the background connectivity check completes.
// A nil err means the provider is reachable.
type editorConnMsg struct {
	// provider and seq identify the check, so a late result for a provider
	// that is no longer active, or for a superseded check, is dropped.
	provider string
	seq      int
	err      error
}

// editorAuthMsg is returned when background auth verification completes in
// the editor. A nil err means the credentials work.
type editorAuthMsg struct {
	err error
}
