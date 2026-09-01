package tools

import "github.com/matteo-psnt/termwise/internal/provider"

// CommandDef defines the command tool. The agent loop intercepts command calls
// and surfaces the proposed command to the user instead of executing it.
var CommandDef = provider.ToolDef{
	Name:        "command",
	Description: "Propose a shell command for the user to review and run.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The shell command to propose.",
			},
		},
		"required": []string{"content"},
	},
}
