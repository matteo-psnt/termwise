package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxBashOutputBytes = 64 * 1024 // 64KB per stream

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
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return fmt.Sprintf("error starting command: %s", err), true
		}
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
