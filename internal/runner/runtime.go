package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/envcontext"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// runtimeContext is the resolved per-invocation environment shared by every
// non-TUI entry point.
type runtimeContext struct {
	client       provider.AgentClient
	providerName string
	modelID      string
	llmJudge     bool
	providerCfg  config.ProviderConfig
	isTTY        bool
	env          envcontext.Context
}

// appendPromptStdin appends piped stdin to the prompt. A terminal stdin or an
// empty pipe leaves the prompt untouched.
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

// writeFinalOutput prints a final agent response. Commands go out raw so they
// can be piped or pasted; text is rendered as markdown on a terminal.
func writeFinalOutput(output agent.FinalResponse, isTTY bool) error {
	if output.Type == "command" {
		fmt.Println(output.Content)
		return nil
	}
	writeRenderedText(output.Content, isTTY)
	return nil
}

// resolveRuntime loads config and returns the client, model, output mode, and
// llm_judge setting for this invocation.
func resolveRuntime() (runtimeContext, error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return runtimeContext{}, err
	}
	cfg, rc, err := config.LoadAndResolve(cfgPath)
	if err != nil {
		return runtimeContext{}, err
	}
	client, err := config.NewClientFromResolved(rc)
	if err != nil {
		return runtimeContext{}, err
	}
	return runtimeContext{
		client:       client,
		providerName: rc.ProviderName,
		modelID:      rc.Model,
		llmJudge:     config.ResolveBoolSetting(cfg, "llm_judge"),
		providerCfg:  cfg.Providers[rc.ProviderName],
		isTTY:        tty.IsTerminal(os.Stdout),
		env:          envcontext.Detect(context.Background(), envcontext.Options{ShellHistory: config.ResolveBoolSetting(cfg, "shell_context")}),
	}, nil
}
