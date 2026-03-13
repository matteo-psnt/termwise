package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
)

var tagRe = regexp.MustCompile(`(?s)<(?:command|text)>(.*?)</(?:command|text)>`)

// ShellWidget handles the --shell-widget invocation used by the Ctrl+T keybinding.
// It always outputs raw text (no glamour, no exit codes) and suppresses errors
// so they never corrupt the shell buffer.
func ShellWidget(ctx context.Context, prompt string) error {
	provider, modelID, err := resolveProvider()
	if err != nil {
		return nil // suppress — never corrupt shell buffer
	}

	resp, err := provider.Complete(ctx, ai.CompleteRequest{
		Model:  modelID,
		System: systemprompt.SingleShot(false), // always treat as piped (raw output)
		Prompt: prompt,
	})
	if err != nil {
		return nil // suppress
	}

	content := resp.Content

	// Strip any XML tags and return the inner content.
	if m := tagRe.FindStringSubmatch(content); m != nil {
		content = strings.TrimSpace(m[1])
	} else {
		content = strings.TrimSpace(content)
	}

	fmt.Println(content)
	return nil
}
