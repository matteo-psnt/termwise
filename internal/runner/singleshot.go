package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/tty"
)

var (
	commandRe = regexp.MustCompile(`(?s)<command>(.*?)</command>`)
	textRe    = regexp.MustCompile(`(?s)<text>(.*?)</text>`)
)

// SingleShot runs a single-shot prompt and writes the response to stdout.
// On a <command> response it returns nil (exit 0).
// On a <text> response it returns ExitCode{10}.
// If stdin is piped its contents are appended to the prompt automatically.
func SingleShot(ctx context.Context, prompt string) error {
	isTTY := tty.IsTerminal(os.Stdout)

	if !tty.IsTerminal(os.Stdin) {
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		if len(stdin) > 0 {
			prompt = prompt + "\n\n" + strings.TrimRight(string(stdin), "\n")
		}
	}

	client, modelID, err := resolveProvider()
	if err != nil {
		return err
	}

	resp, err := client.Complete(ctx, provider.CompleteRequest{
		Model:  modelID,
		System: systemprompt.SingleShot(isTTY),
		Prompt: prompt,
	})
	if err != nil {
		return err
	}

	return writeOutput(resp.Content, isTTY)
}

// writeOutput parses the model's response and writes it to stdout.
func writeOutput(content string, isTTY bool) error {
	if m := commandRe.FindStringSubmatch(content); m != nil {
		fmt.Println(strings.TrimSpace(m[1]))
		return nil // exit 0
	}

	text := content
	if m := textRe.FindStringSubmatch(content); m != nil {
		text = strings.TrimSpace(m[1])
	} else {
		text = strings.TrimSpace(content)
	}

	if isTTY {
		rendered, err := glamour.Render(text, "auto")
		if err == nil {
			fmt.Print(rendered)
			return ExitCode{10}
		}
	}
	fmt.Println(text)
	return ExitCode{10}
}

// resolveProvider loads config and returns a ready provider client and model ID.
func resolveProvider() (provider.AgentClient, string, error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return nil, "", err
	}
	rc, err := config.LoadRuntimeConfig(cfgPath)
	if err != nil {
		return nil, "", err
	}
	client, err := config.NewClientFromResolved(rc)
	if err != nil {
		return nil, "", err
	}
	return client, rc.Model, nil
}
