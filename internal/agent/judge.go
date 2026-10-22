package agent

import (
	"context"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const judgeSystemPrompt = `You are a safety classifier for bash commands. Respond with exactly one word: safe or unsafe.

A command is SAFE only if it is either strictly read-only or writes only transient scratch output to a temp location.
Examples of SAFE commands: ls, cat, grep, git status, git log, git diff, find, ps, df, du, echo, pwd, printf ok > /tmp/out.txt, rg TODO . > /tmp/matches.txt.
Temp-write SAFE commands are limited to obvious temp roots such as /tmp, /private/tmp, or the system temp directory. Treat relative paths, ~ paths, and anything not plainly inside those temp roots as unsafe.

A command is UNSAFE if it could modify files, git state, processes, network state, or any system state.
Examples of UNSAFE commands: git reset, git commit, git push, git checkout, rm, mv, cp, mkdir, chmod, kill, curl, wget, printf ok > out.txt, printf ok > ~/out.txt, any command with a write flag outside approved temp locations.

When in doubt, respond unsafe.`

// JudgeBashCommand asks the configured model whether a bash command is safe to
// auto-execute without interactive confirmation.
func JudgeBashCommand(ctx context.Context, client provider.AgentClient, modelID string, command string) (bool, error) {
	resp, err := client.Chat(ctx, provider.ChatRequest{
		Model:  modelID,
		System: judgeSystemPrompt,
		Messages: []provider.Message{
			{Role: "user", Content: "Is this command read-only and safe to auto-execute?\n\n" + command},
		},
	})
	if err != nil {
		return false, err
	}
	return isSafeVerdict(resp.Content), nil
}

// isSafeVerdict reports whether the judge's reply is the single word "safe".
// Models often add a full stop or markdown emphasis ("Safe.", "**safe**"), and
// rejecting those would gate commands the judge approved; anything more than
// the bare word still counts as unsafe.
func isSafeVerdict(reply string) bool {
	word := strings.Trim(strings.ToLower(reply), " \t\r\n.!*_`'\"")
	return word == "safe"
}
