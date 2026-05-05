package tools

import "github.com/matteo-psnt/termwise/internal/provider"

// Defs are the tool definitions sent to the model in every agent request.
var Defs = []provider.ToolDef{
	{
		Name:        "read",
		Description: "Read a file's contents. Supports offset and limit for large files.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Line number to start from (default: 0).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max lines to return (default: 2000, max: 2000).",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "bash",
		Description: "Execute a shell command. Returns stdout, stderr, and exit code as JSON.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute.",
				},
			},
			"required": []string{"command"},
		},
	},
	{
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
	},
	{
		Name:        "respond",
		Description: "Deliver the final response to the user. Use 'command' for runnable shell commands, 'text' for explanations.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"command", "text"},
					"description": "'command' for a shell command to run, 'text' for an explanation or answer.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The response content.",
				},
			},
			"required": []string{"type", "content"},
		},
	},
}
