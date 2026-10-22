package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const maxBashOutputBytes = 64 * 1024 // 64KB per stream

func init() {
	stepFns[BashDef.Name] = func(tc provider.ToolCall) Step {
		return Step{Kind: StepGatedExecutor, GateCommand: BashCommand(tc), Execute: ExecuteBash}
	}
	detailFns[BashDef.Name] = BashCommand
	displayFns[BashDef.Name] = formatDisplay
}

var BashDef = provider.ToolDef{
	Name: "bash",
	Description: `Execute a read-only shell command to find something out. Returns stdout, stderr, and exit code as JSON.

This is how you check facts instead of guessing. Reach for it before proposing anything you are not certain of:
- Does a package exist, and what is it called?  brew search <name> / npm view <name> version
- Where did a binary come from?                 which -a <cmd>
- What is actually running / listening?         lsof -i :<port> / ps aux | grep <name>
- What is the repo state?                       git status --porcelain / git log --oneline -5

Read-only commands run automatically. Anything that modifies files, git state, processes, or system state is gated behind user approval and will interrupt them — so do not use bash to perform actions. Propose those with the command tool instead.`,
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
}

// BashCommand returns the command argument for a bash tool call.
func BashCommand(tc provider.ToolCall) string {
	return stringInput(tc.Input, "command")
}

// ExecuteBash runs the bash tool for a tool call and packages the result.
// It stays a named function because the approval path outside this package runs
// bash directly once the user confirms.
func ExecuteBash(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	return executor(bash)(ctx, tc)
}

// bashResult is the structured output returned by the bash tool.
type bashResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// bash executes a shell command and returns the result as a JSON string.
// Returns (content, isError).
func bash(ctx context.Context, input map[string]any) (string, bool) {
	command := stringInput(input, "command")
	if command == "" {
		return "error: 'command' is required", true
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, commandShell(), "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Sprintf("error starting command: %s", err), true
		}
		exitCode = exitErr.ExitCode()
	}

	result := bashResult{
		Stdout:   truncateBashOutput(stdout.String()),
		Stderr:   truncateBashOutput(stderr.String()),
		ExitCode: exitCode,
	}
	data, _ := json.Marshal(result)
	return string(data), exitCode != 0
}

// formatDisplay formats a bash tool result JSON string for human display,
// returning just stdout and stderr without the JSON wrapper or exit code.
func formatDisplay(jsonContent string) string {
	var result bashResult
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return jsonContent
	}
	var parts []string
	if s := strings.TrimRight(result.Stdout, "\n"); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimRight(result.Stderr, "\n"); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

func truncateBashOutput(s string) string {
	if len(s) <= maxBashOutputBytes {
		return s
	}
	return s[:maxBashOutputBytes] + "\n[output truncated — 64KB limit]"
}

// commandShell is the shell the bash tool runs commands in. It has to be the
// language the allowlist parses (bash), not the user's $SHELL: the approval
// decision is made on a bash reading of the text, and fish, nu or zsh can
// read the same text differently, so a command judged read-only could do
// something else when it runs. /bin/sh is the fallback on the rare system
// with no bash; the parser's bash dialect is a superset of what it accepts.
func commandShell() string {
	if path, err := exec.LookPath("bash"); err == nil {
		return path
	}
	return "/bin/sh"
}
