package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
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

// A command must run without termwise's controlling terminal, so a read from
// /dev/tty fails at once instead of stopping the command with SIGTTIN.
func TestBashCommandHasNoControllingTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, isErr := bash(ctx, map[string]any{"command": `if : </dev/tty 2>/dev/null; then echo tty; else echo none; fi`})
	if isErr {
		t.Fatalf("bash tool failed: %s", out)
	}
	if !strings.Contains(out, "none") {
		t.Errorf("command could open /dev/tty: %s", out)
	}
}

func runBashResult(ctx context.Context, t *testing.T, command string) (bashResult, string, bool) {
	t.Helper()
	out, isErr := bash(ctx, map[string]any{"command": command})
	var res bashResult
	_ = json.Unmarshal([]byte(out), &res)
	return res, out, isErr
}

// A command that succeeds but leaves a background child holding its output
// is a success with the output it produced, not an error.
func TestBashKeepsOutputWhenABackgroundChildOutlivesIt(t *testing.T) {
	res, out, isErr := runBashResult(context.Background(), t, `echo hi; sleep 3 &`)
	if isErr || res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "hi" {
		t.Fatalf("got isErr=%v %s, want stdout hi and exit 0", isErr, out)
	}
}

// Endless output is stopped at the cap instead of running until cancelled.
func TestBashStopsACommandPastTheOutputCap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	res, out, _ := runBashResult(ctx, t, `yes`)
	if ctx.Err() != nil {
		t.Fatal("yes ran until the test deadline instead of being stopped at the cap")
	}
	if len(res.Stdout) > maxBashOutputBytes+100 || !strings.Contains(res.Stderr, "64KB") {
		t.Errorf("unexpected result after %v: %.200s", time.Since(start), out)
	}
}

func TestBashReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	_, out, isErr := runBashResult(ctx, t, `sleep 5`)
	if !isErr || !strings.Contains(out, "cancelled") {
		t.Errorf("cancelled command returned isErr=%v %q", isErr, out)
	}
}
