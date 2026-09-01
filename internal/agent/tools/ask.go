package tools

import "github.com/matteo-psnt/termwise/internal/provider"

func init() {
	stepFns[AskDef.Name] = func(tc provider.ToolCall) Step {
		q, opts, multi := AskInput(tc)
		return Step{Kind: StepAsk, Question: q, Options: opts, MultiSelect: multi}
	}
}

// AskInput extracts the question, options, and multi_select flag from an ask
// tool call.
func AskInput(tc provider.ToolCall) (question string, options []string, multiSelect bool) {
	question, _ = tc.Input["question"].(string)
	if opts, ok := tc.Input["options"].([]any); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok {
				options = append(options, s)
			}
		}
	}
	multiSelect, _ = tc.Input["multi_select"].(bool)
	return question, options, multiSelect
}

// AskDef defines the ask tool. The tool has no executor in this package — the
// agent loop turns ask calls into a user prompt and supplies the user's choice
// as the tool result.
var AskDef = provider.ToolDef{
	Name: "ask",
	Description: "Ask the user a structured question with a list of options. " +
		"Use this when there are multiple valid paths forward and the user's preference matters.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to present to the user.",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Options for the user to choose from.",
			},
			"multi_select": map[string]any{
				"type":        "boolean",
				"description": "Allow selecting multiple options. Default: false.",
			},
		},
		"required": []string{"question", "options"},
	},
}
