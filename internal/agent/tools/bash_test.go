package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBashMarksNonZeroExitAsError(t *testing.T) {
	content, isError := bash(context.Background(), map[string]any{"command": "exit 7"})
	if !isError {
		t.Fatalf("expected non-zero exit to be marked as an error")
	}

	var result bashResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal bash result: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
}

func TestBashZeroExitIsNotError(t *testing.T) {
	content, isError := bash(context.Background(), map[string]any{"command": "printf ok"})
	if isError {
		t.Fatalf("expected zero exit to be treated as success")
	}

	var result bashResult
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

func TestBashHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _ = bash(ctx, map[string]any{"command": "sleep 5"})
	if time.Since(start) > time.Second {
		t.Fatal("expected canceled bash command to return quickly")
	}
}

// The allowlist judges commands as bash, so they must run in bash whatever
// the user's login shell is.
func TestBashRunsInBashNotUserShell(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	t.Setenv("SHELL", "/bin/false")
	out, isErr := bash(context.Background(), map[string]any{"command": `echo "$BASH_VERSION"`})
	if isErr {
		t.Fatalf("bash tool failed: %s", out)
	}
	var res bashResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unexpected output %q: %v", out, err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Errorf("command did not run in bash: %+v", res)
	}
}

// Cancelling must stop the whole command, not just bash: a child that
// inherits stdout (here the sleep) would otherwise keep Run waiting, and keep
// running, after the user pressed Esc.
func TestBashCancellationStopsChildren(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _ = bash(ctx, map[string]any{"command": "sleep 5; echo done"})
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("cancelled command returned after %v", d)
	}
}
