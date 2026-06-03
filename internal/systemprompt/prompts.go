package systemprompt

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const singleShotTemplate = `You are a terminal assistant. You help with command generation and quick questions about CLI tools, terminal workflows, and project-related topics.

Return ONLY the requested output wrapped in a type tag.

Response format:
- Shell commands: <command>your command here</command>
- Answers, explanations, information: <text>your response here</text>

Rules:
- No preamble or commentary outside the tags
- Only one tag per response
- If the user describes an intent or asks "how do I...", return a <command>
- If the user asks a question, return a <text> answer — keep it concise
- For commands, return the raw command inside the tag — no markdown
- For text output, use markdown formatting when it improves readability
- When output is piped, never use markdown inside <text> — plain text only
- If the user provides input context (piped stdin), use it to inform your response

Environment:
- OS: %s
- Shell: %s
- Output: %s`

const agentTemplate = `You are a terminal assistant with access to tools. Help the user by reading files, searching code, and running commands.

Rules:
- Use tools to gather information before answering when helpful
- Prefer read-only commands. Only suggest writes when the user asks for changes.
- Be concise in your responses
- When showing results, use markdown formatting when it improves readability
- %s
- Prefer using tools to find answers over asking the user
- You MUST use the respond tool to deliver your final answer. Never return bare text.

Available tools: %s

Environment:
- OS: %s
- Shell: %s
- Working directory: %s`

// SingleShot returns the single-shot system prompt with environment context injected.
// isTTY controls whether the output context is "terminal" or "piped".
func SingleShot(isTTY bool) string {
	output := "terminal"
	if !isTTY {
		output = "piped"
	}
	return fmt.Sprintf(singleShotTemplate, osName(), shellPath(), output)
}

// Agent returns the agent-mode system prompt with environment context injected.
// The available tool list is derived from the provided tool definitions.
func Agent(toolDefs []provider.ToolDef) string {
	return agentWithTools(provider.ToolNames(toolDefs))
}

func agentWithTools(toolNames []string) string {
	cwd, _ := os.Getwd()
	return fmt.Sprintf(agentTemplate, askToolGuidance(toolNames), strings.Join(toolNames, ", "), osName(), shellPath(), cwd)
}

func askToolGuidance(toolNames []string) string {
	for _, name := range toolNames {
		if name == "ask" {
			return "Use the ask tool when there are multiple valid paths and the user's preference matters"
		}
	}
	return "If user clarification would help, explain the ambiguity in your final response instead of asking follow-up questions"
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func shellPath() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}
