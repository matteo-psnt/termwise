package systemprompt

import (
	"strings"
	"testing"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/envcontext"
)

// testEnv is a fixed environment so prompt assertions don't depend on the
// machine the tests happen to run on.
var testEnv = envcontext.Context{
	OS:       "macOS",
	Shell:    "/bin/zsh",
	WorkDir:  "/tmp/proj",
	Project:  []string{"Go (go.mod)"},
	PkgMgrs:  []string{"brew", "npm"},
	CLITools: []string{"rg", "git"},
}

func TestAgentPromptListsAvailableTools(t *testing.T) {
	withAsk := Agent(agenttools.Defs, testEnv)
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
	headless := AgentHeadless(agenttools.HeadlessDefs, testEnv)
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

func TestAgentPromptCarriesEnvironment(t *testing.T) {
	p := Agent(agenttools.Defs, testEnv)
	for _, want := range []string{
		"Package managers installed: brew, npm",
		"CLI tools available: rg, git",
		"Project: Go (go.mod)",
		"/tmp/proj",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// The Gemini and codex sessions both failed because the model guessed a package
// name and an install source instead of checking. The policy has to be present.
func TestAgentPromptCarriesVerifyPolicy(t *testing.T) {
	p := Agent(agenttools.Defs, testEnv)
	for _, want := range []string{
		"brew search <name>",
		"which -a <cmd>",
		"Do not use the ask tool for anything a read-only command could answer",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing verify-policy line %q", want)
		}
	}
}
