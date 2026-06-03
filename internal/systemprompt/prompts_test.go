package systemprompt

import (
	"strings"
	"testing"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
)

func TestAgentPromptMentionsAskToolOnlyWhenAvailable(t *testing.T) {
	withAsk := Agent(agenttools.Defs)
	if !strings.Contains(withAsk, "Use the ask tool when there are multiple valid paths") {
		t.Fatalf("expected interactive prompt to mention ask tool, got:\n%s", withAsk)
	}

	headless := Agent(agenttools.HeadlessDefs)
	if strings.Contains(headless, "Use the ask tool when there are multiple valid paths") {
		t.Fatalf("expected headless prompt to omit ask-tool guidance, got:\n%s", headless)
	}
	if !strings.Contains(headless, "instead of asking follow-up questions") {
		t.Fatalf("expected headless prompt to explain non-interactive clarification behavior, got:\n%s", headless)
	}
}
