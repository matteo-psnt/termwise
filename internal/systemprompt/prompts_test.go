package systemprompt

import (
	"strings"
	"testing"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
)

func TestAgentPromptListsAvailableTools(t *testing.T) {
	withAsk := Agent(agenttools.Defs)
	if !strings.Contains(withAsk, agenttools.AskDef.Name) {
		t.Fatalf("expected interactive prompt to list the ask tool, got:\n%s", withAsk)
	}
	if !strings.Contains(withAsk, "/tmp, /private/tmp, or the system temp directory") {
		t.Fatalf("expected interactive prompt to mention allowed temp-file writes, got:\n%s", withAsk)
	}

	// Interactive prompt must NOT carry the non-interactive clarification rule.
	if strings.Contains(withAsk, "non-interactive") {
		t.Fatalf("expected interactive prompt to omit the non-interactive rule, got:\n%s", withAsk)
	}
}

func TestAgentHeadlessPromptAddsNonInteractiveRule(t *testing.T) {
	headless := AgentHeadless(agenttools.HeadlessDefs)
	if !strings.Contains(headless, "non-interactive") {
		t.Fatalf("expected headless prompt to flag non-interactive mode, got:\n%s", headless)
	}
	if !strings.Contains(headless, "do not ask follow-up questions") {
		t.Fatalf("expected headless prompt to forbid follow-up questions, got:\n%s", headless)
	}
	if !strings.Contains(headless, "/tmp, /private/tmp, or the system temp directory") {
		t.Fatalf("expected headless prompt to mention allowed temp-file writes, got:\n%s", headless)
	}
}

func TestAskToolDescriptionCarriesWhenToUseGuidance(t *testing.T) {
	// The "when to use ask" policy moved from the system prompt into the
	// tool description, which the model sees per turn via the API.
	if !strings.Contains(agenttools.AskDef.Description, "multiple valid paths") {
		t.Fatalf("expected ask tool description to carry when-to-use guidance, got:\n%s", agenttools.AskDef.Description)
	}
}
