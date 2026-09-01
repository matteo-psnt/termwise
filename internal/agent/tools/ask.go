package tools

import "github.com/matteo-psnt/termwise/internal/provider"

// AskDef defines the ask tool. The tool has no executor in this package — the
// agent loop turns ask calls into a user prompt and supplies the user's choice
// as the tool result.
var AskDef = provider.ToolDef{
	Name:        "ask",
	Description: "Ask the user a structured question with a list of options.",
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
