package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/tty"
)

var (
	commandRe = regexp.MustCompile(`(?s)<command>(.*?)</command>`)
	textRe    = regexp.MustCompile(`(?s)<text>(.*?)</text>`)
)

type runtimeContext struct {
	client   provider.AgentClient
	modelID  string
	llmJudge bool
	isTTY    bool
}

// SingleShot runs a single-shot prompt and writes the response to stdout.
// On a <command> response it returns nil (exit 0).
// On a <text> response it returns ExitCode{10}.
// If stdin is piped its contents are appended to the prompt automatically.
func SingleShot(ctx context.Context, prompt string) error {
	var err error
	prompt, err = appendPromptStdin(prompt)
	if err != nil {
		return err
	}

	rt, err := resolveRuntime()
	if err != nil {
		return err
	}

	resp, err := rt.client.Complete(ctx, provider.CompleteRequest{
		Model:  rt.modelID,
		System: systemprompt.SingleShot(rt.isTTY),
		Prompt: prompt,
	})
	if err != nil {
		return err
	}

	return writeFinalOutput(parseTaggedOutput(resp.Content), rt.isTTY, ExitCode{10})
}

func appendPromptStdin(prompt string) (string, error) {
	if tty.IsTerminal(os.Stdin) {
		return prompt, nil
	}

	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	if len(stdin) == 0 {
		return prompt, nil
	}
	return prompt + "\n\n" + strings.TrimRight(string(stdin), "\n"), nil
}

func parseTaggedOutput(content string) agent.FinalResponse {
	if m := commandRe.FindStringSubmatch(content); m != nil {
		return agent.FinalResponse{Type: "command", Content: strings.TrimSpace(m[1])}
	}

	var text string
	if m := textRe.FindStringSubmatch(content); m != nil {
		text = strings.TrimSpace(m[1])
	} else {
		text = strings.TrimSpace(content)
	}
	return agent.FinalResponse{Type: "text", Content: text}
}

func writeRenderedText(text string, isTTY bool) {
	if isTTY {
		rendered, err := glamour.Render(text, "auto")
		if err == nil {
			fmt.Print(rendered)
			return
		}
	}
	fmt.Println(text)
}

// writeFinalOutput writes a final response and optionally returns textExitErr for text responses.
func writeFinalOutput(output agent.FinalResponse, isTTY bool, textExitErr error) error {
	if output.Type == "command" {
		fmt.Println(output.Content)
		return nil
	}
	writeRenderedText(output.Content, isTTY)
	return textExitErr
}

// resolveRuntime loads config and returns the runtime client, model, output mode, and llm_judge setting.
func resolveRuntime() (runtimeContext, error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return runtimeContext{}, err
	}
	rc, err := config.LoadRuntimeConfig(cfgPath)
	if err != nil {
		return runtimeContext{}, err
	}
	client, err := config.NewClientFromResolved(rc)
	if err != nil {
		return runtimeContext{}, err
	}

	llmJudge := false
	if cfg, exists, err := config.LoadConfig(cfgPath); err == nil && exists {
		llmJudge = cfg.Settings.LLMJudge
	}
	return runtimeContext{
		client:   client,
		modelID:  rc.Model,
		llmJudge: llmJudge,
		isTTY:    tty.IsTerminal(os.Stdout),
	}, nil
}
