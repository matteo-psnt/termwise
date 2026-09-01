package tools

import (
	"context"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// StepKind describes how the agent loop should handle a tool call. The kinds
// reflect genuinely-different dispatch shapes (execute, gated-execute, respond,
// ask), not individual tool identities.
type StepKind int

const (
	// StepExecutor runs immediately and returns its result. Used by read-only
	// tools like read, grep, glob.
	StepExecutor StepKind = iota
	// StepGatedExecutor requires user approval (via needsApproval) before
	// executing. Used by bash.
	StepGatedExecutor
	// StepRespond surfaces Content to the user as a final response and ends
	// the agent turn. Used by command.
	StepRespond
	// StepAsk presents a question with options to the user and resumes the
	// loop with the user's selection. Used by ask.
	StepAsk
)

// Step is a tool-call-specific dispatch intent built by the tool's registered
// stepFn. The loop reads Kind and uses the relevant fields.
type Step struct {
	Kind        StepKind
	Execute     func(ctx context.Context, tc provider.ToolCall) provider.ToolResult
	GateCommand string
	Content     string
	Question    string
	Options     []string
	MultiSelect bool
}

// Registration tables populated by each tool file's init().
var (
	stepFns    = map[string]func(provider.ToolCall) Step{}
	detailFns  = map[string]func(provider.ToolCall) string{}
	displayFns = map[string]func(string) string{}
)

// StepFor returns the dispatch intent for a tool call. ok=false when the tool
// name is not registered (unknown tool).
func StepFor(tc provider.ToolCall) (Step, bool) {
	fn, ok := stepFns[tc.Name]
	if !ok {
		return Step{}, false
	}
	return fn(tc), true
}

// Detail returns the UI summary line for a tool call (e.g. the bash command,
// the read path, the grep pattern). Empty string for tools without a registered
// formatter.
func Detail(tc provider.ToolCall) string {
	if fn, ok := detailFns[tc.Name]; ok {
		return fn(tc)
	}
	return ""
}

// DisplayResult returns a UI-formatted version of a tool result's content.
// Tools that need to unwrap structured output (e.g. bash JSON) register a
// formatter; others fall through to the raw content.
func DisplayResult(name, content string) string {
	if fn, ok := displayFns[name]; ok {
		return fn(content)
	}
	return content
}
