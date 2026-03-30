package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
func Bash(input map[string]any) (string, bool) {
	command, _ := input["command"].(string)
	if command == "" {
		return "error: 'command' is required", true
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(shell, "-c", command)
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

func truncateBashOutput(s string) string {
	if len(s) <= maxBashOutputBytes {
		return s
	}
	return s[:maxBashOutputBytes] + "\n[output truncated — 64KB limit]"
}
