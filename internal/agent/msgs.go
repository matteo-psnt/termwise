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
	RespondType  string
	Content      string
	AutoAccepted bool
}

// ResponseMsg is sent when the model returns a chat response.
type ResponseMsg struct {
	Resp *provider.ChatResponse
	Err  error
}

// ToolExecutedMsg is sent when a tool (read or auto-accepted bash) has been executed.
type ToolExecutedMsg struct {
	ToolCall     provider.ToolCall
	Result       provider.ToolResult
	Remaining    []provider.ToolCall
	Collected    []provider.ToolResult
	AutoAccepted bool
}

// NeedsApprovalMsg is sent when a bash command requires user approval before running.
type NeedsApprovalMsg struct {
	ToolCall  provider.ToolCall
	Command   string
	Remaining []provider.ToolCall
	Collected []provider.ToolResult
}

// AskMsg is sent when the model uses the ask tool.
type AskMsg struct {
	ToolCall    provider.ToolCall
	Question    string
	Options     []string
	MultiSelect bool
	Remaining   []provider.ToolCall
	Collected   []provider.ToolResult
}

// RespondMsg is sent when the model calls the respond tool.
type RespondMsg struct {
	ToolCallID  string
	RespondType string // "command" | "text"
	Content     string
	Collected   []provider.ToolResult
}

// AllToolsDoneMsg is sent when all tool calls have been processed and results
// should be sent back to the model.
type AllToolsDoneMsg struct {
	Collected []provider.ToolResult
}
