package agent

import "github.com/matteo-psnt/termwise/internal/provider"

// ToolStepKind describes the outcome of processing the next tool call.
type ToolStepKind int

const (
	ToolStepRespond ToolStepKind = iota
	ToolStepExecuted
	ToolStepNeedsApproval
	ToolStepAsk
	ToolStepDone
)

// ToolStep is the reusable, non-UI representation of the next tool-processing step.
type ToolStep struct {
	Kind         ToolStepKind
	ToolCall     provider.ToolCall
	Result       provider.ToolResult
	Remaining    []provider.ToolCall
	Collected    []provider.ToolResult
	Command      string
	Question     string
	Options      []string
	MultiSelect  bool
	Content      string
	AutoAccepted bool
}

// ResponseEvent reports a chat response from the model.
type ResponseEvent struct {
	Resp *provider.ChatResponse
	Err  error
}

// ToolExecutedEvent reports that a tool (read or auto-accepted bash) has been executed.
type ToolExecutedEvent struct {
	ToolCall     provider.ToolCall
	Result       provider.ToolResult
	Remaining    []provider.ToolCall
	Collected    []provider.ToolResult
	AutoAccepted bool
}

// NeedsApprovalEvent reports that a bash command requires user approval before running.
type NeedsApprovalEvent struct {
	ToolCall  provider.ToolCall
	Command   string
	Remaining []provider.ToolCall
	Collected []provider.ToolResult
}

// AskEvent reports that the model used the ask tool.
type AskEvent struct {
	ToolCall    provider.ToolCall
	Question    string
	Options     []string
	MultiSelect bool
	Remaining   []provider.ToolCall
	Collected   []provider.ToolResult
}

// RespondEvent reports that the model called the command tool.
type RespondEvent struct {
	ToolCallID string
	Content    string
	Collected  []provider.ToolResult
}

// AllToolsDoneEvent reports that all tool calls have been processed and results
// should be sent back to the model.
type AllToolsDoneEvent struct {
	Collected []provider.ToolResult
}
