package systemprompt

import (
	"fmt"
	"os"
	"runtime"
)

const agentTemplate = `You are a terminal assistant with access to tools. Help the user by reading files, searching code, and running commands.

Rules:
- Use tools to gather information before answering when helpful
- Prefer read-only commands. Only suggest writes when the user asks for changes.
- Be concise in your responses
- When showing results, use markdown formatting when it improves readability
- Use the ask tool when there are multiple valid paths and the user's preference matters
- Prefer using tools to find answers over asking the user
- You MUST use the respond tool to deliver your final answer. Never return bare text.

Available tools: read, bash, ask, respond

Environment:
- OS: %s
- Shell: %s
- Working directory: %s`

// Agent returns the agent mode system prompt with environment context injected.
func Agent() string {
	cwd, _ := os.Getwd()
	return fmt.Sprintf(agentTemplate, osName(), shellPath(), cwd)
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
