package tools

import (
	"strings"
	"testing"
)

func TestAskToolDescriptionCarriesWhenToUseGuidance(t *testing.T) {
	// The "when to use ask" policy moved from the system prompt into the
	// tool description, which the model sees per turn via the API.
	if !strings.Contains(AskDef.Description, "multiple valid paths") {
		t.Fatalf("expected ask tool description to carry when-to-use guidance, got:\n%s", AskDef.Description)
	}
}
