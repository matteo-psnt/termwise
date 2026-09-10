package tools

import "github.com/matteo-psnt/termwise/internal/provider"

func init() {
	stepFns[CommandDef.Name] = func(tc provider.ToolCall) Step {
		return Step{Kind: StepRespond, Content: CommandContent(tc)}
	}
}

// CommandContent returns the proposed command body from a command tool call.
func CommandContent(tc provider.ToolCall) string {
	c, _ := tc.Input["content"].(string)
	return c
}

// CommandDef defines the command tool. The agent loop intercepts command calls
// and surfaces the proposed command to the user instead of executing it.
var CommandDef = provider.ToolDef{
	Name: "command",
	Description: `Propose a shell command for the user to review and run. This is the primary output of this assistant — when the user describes something they want done, this tool is the answer, not a prose explanation.

USE THIS TOOL WHEN the user describes an action, an intent, or asks "how do I…". Phrases like "give me the command", "show me", "run", "install", "kill", "find", "how do I" all mean: propose a command.

THE COMMAND MUST BE READY TO RUN AS-IS:
- Exactly one command. Pipes and && are fine; two unrelated commands are not.
- No placeholders. Never <file>, YOUR_KEY, /path/to/x, or "…". Fill in the real path, port, branch, or name from the conversation and the environment. If you do not know a value, find it with a read-only bash call first.
- Only tools the user actually has. The environment block lists the installed package managers and CLI tools — do not propose one that is missing.
- No prose, no markdown fences, no leading $ or %. The content field is the command text and nothing else. Explanation goes in your text response, not in here.
- No sudo unless the operation genuinely requires root.

EXAMPLES:
"undo last commit but keep changes"  -> git reset --soft HEAD~1
"whats running on port 5173"         -> lsof -i :5173
"kill whatever is on port 5173"      -> kill $(lsof -ti :5173)
"list files by size"                 -> ls -lhS
"largest file in the project"        -> find . -type f -exec du -h {} + | sort -hr | head -1
"folders ranked by size"             -> du -sh */ | sort -hr

DESTRUCTIVE OPERATIONS: if the command deletes, overwrites, force-pushes, or kills a process, still propose it — but say what it will affect in your text response first.`,
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
