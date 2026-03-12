package agent

import "github.com/matteo-psnt/termwise/internal/ai"

// ResponseMsg is sent when the model returns a chat response.
type ResponseMsg struct {
	Resp *ai.ChatResponse
	Err  error
}

// ToolExecutedMsg is sent when a tool (read or auto-accepted bash) has been executed.
type ToolExecutedMsg struct {
	ToolCall     ai.ToolCall
	Result       ai.ToolResult
	Remaining    []ai.ToolCall
	Collected    []ai.ToolResult
	AutoAccepted bool
}

// NeedsApprovalMsg is sent when a bash command requires user approval before running.
type NeedsApprovalMsg struct {
	ToolCall  ai.ToolCall
	Command   string
	Remaining []ai.ToolCall
	Collected []ai.ToolResult
}

// AskMsg is sent when the model uses the ask tool.
type AskMsg struct {
	ToolCall    ai.ToolCall
	Question    string
	Options     []string
	MultiSelect bool
	Remaining   []ai.ToolCall
	Collected   []ai.ToolResult
}

// RespondMsg is sent when the model calls the respond tool.
type RespondMsg struct {
	ToolCallID  string
	RespondType string // "command" | "text"
	Content     string
	Collected   []ai.ToolResult
}

// AllToolsDoneMsg is sent when all tool calls have been processed and results
// should be sent back to the model.
type AllToolsDoneMsg struct {
	Collected []ai.ToolResult
}
