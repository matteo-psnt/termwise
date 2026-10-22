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
