package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const maxBashOutputBytes = 64 * 1024 // 64KB per stream

// bashWaitDelay is how long a cancelled or finished command's output pipes may
// stay open before Run gives up on them.
const bashWaitDelay = 500 * time.Millisecond

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

	// Stop the command once it has produced more output than can be shown:
	// past the cap the output would be thrown away anyway, and an endless
	// writer (cat /dev/zero, yes) would otherwise run, and grow memory, until
	// the user noticed.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	stdout := &cappedBuffer{max: maxBashOutputBytes, onFull: stop}
	stderr := &cappedBuffer{max: maxBashOutputBytes, onFull: stop}

	cmd := exec.CommandContext(runCtx, commandShell(), "-c", command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	killProcessGroup(cmd)
	// Backstop for anything that escaped the group (e.g. a daemon that
	// called setsid) but still holds the output pipes.
	cmd.WaitDelay = bashWaitDelay

	err := cmd.Run()
	if ctx.Err() != nil {
		return "error: command cancelled", true
	}
	exitCode := 0
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	case errors.Is(err, exec.ErrWaitDelay):
		// The command itself finished; a background child it started was
		// still holding the output pipes when WaitDelay closed them. The
		// output so far and bash's own exit status are the result.
		exitCode = cmd.ProcessState.ExitCode()
	default:
		return fmt.Sprintf("error starting command: %s", err), true
	}

	result := bashResult{
		Stdout:   stdout.text(),
		Stderr:   stderr.text(),
		ExitCode: exitCode,
	}
	if stdout.full || stderr.full {
		result.Stderr += "\n[command stopped: output passed the 64KB limit]"
	}
	data, _ := json.Marshal(result)
	return string(data), exitCode != 0
}

// cappedBuffer keeps the first max bytes written to it and calls onFull once
// when more arrive. It never returns an error, so the writer is not killed
// by EPIPE before onFull has stopped it cleanly.
type cappedBuffer struct {
	buf    bytes.Buffer
	max    int
	full   bool
	onFull func()
	mu     sync.Mutex
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := min(max(b.max-b.buf.Len(), 0), len(p))
	b.buf.Write(p[:kept])
	if kept < len(p) && !b.full {
		b.full = true
		if b.onFull != nil {
			b.onFull()
		}
	}
	return len(p), nil
}

// text returns what was kept, marked when the rest was dropped.
func (b *cappedBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.full {
		return b.buf.String() + "\n[output truncated — 64KB limit]"
	}
	return b.buf.String()
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
