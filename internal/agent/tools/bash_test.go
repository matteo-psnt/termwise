package tools

import (
	"encoding/json"
	"testing"
)

func TestBashMarksNonZeroExitAsError(t *testing.T) {
	content, isError := Bash(map[string]any{"command": "exit 7"})
	if !isError {
		t.Fatalf("expected non-zero exit to be marked as an error")
	}

	var result BashResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal bash result: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
}

func TestBashZeroExitIsNotError(t *testing.T) {
	content, isError := Bash(map[string]any{"command": "printf ok"})
	if isError {
		t.Fatalf("expected zero exit to be treated as success")
	}

	var result BashResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal bash result: %v", err)
	}
	if result.Stdout != "ok" {
		t.Fatalf("expected stdout %q, got %q", "ok", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}
