package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	displayFns[BashDef.Name] = FormatDisplay
}

var BashDef = provider.ToolDef{
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
}

// BashCommand returns the command argument for a bash tool call.
func BashCommand(tc provider.ToolCall) string {
	c, _ := tc.Input["command"].(string)
	return c
}

// ExecuteBash runs the bash tool for a tool call and packages the result.
func ExecuteBash(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	content, isErr := Bash(ctx, tc.Input)
	return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

// BashResult is the structured output returned by the bash tool.
type BashResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Bash executes a shell command and returns the result as a JSON string.
// Returns (content, isError).
func Bash(ctx context.Context, input map[string]any) (string, bool) {
	command, _ := input["command"].(string)
	if command == "" {
		return "error: 'command' is required", true
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, shell, "-c", command)
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

	result := BashResult{
		Stdout:   truncateBashOutput(stdout.String()),
		Stderr:   truncateBashOutput(stderr.String()),
		ExitCode: exitCode,
	}
	data, _ := json.Marshal(result)
	return string(data), exitCode != 0
}

// FormatDisplay formats a bash tool result JSON string for human display,
// returning just stdout and stderr without the JSON wrapper or exit code.
func FormatDisplay(jsonContent string) string {
	var result BashResult
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
