package runner

import (
	"context"
	"fmt"
	"os"

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
	// effort is the provider's configured effort, before model defaults are
	// applied; see config.EffectiveEffort.
	effort string
	// cfgPath is the config file in use; the exchange log lives beside it.
	cfgPath string
	isTTY   bool
	env     envcontext.Context
}

// appendPromptStdin appends piped stdin to the prompt. A terminal stdin or an
// empty pipe leaves the prompt untouched.
func appendPromptStdin(prompt string) (string, error) {
	stdin, piped, err := tty.ReadPiped(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	if !piped || stdin == "" {
		return prompt, nil
	}
	return prompt + "\n\n" + stdin, nil
}

// writeFinalOutput prints a final agent response. Commands go out raw so they
// can be piped or pasted; text is rendered as markdown on a terminal, falling
// back to plain text if rendering fails.
func writeFinalOutput(output agent.FinalResponse, isTTY bool) {
	if output.Type != "command" && isTTY {
		if rendered, err := glamour.Render(output.Content, "auto"); err == nil {
			fmt.Print(rendered)
			return
		}
	}
	fmt.Println(output.Content)
}

// resolveRuntime loads config and returns the client, model, settings and
// detected environment for this invocation.
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
		effort:       cfg.Providers[rc.ProviderName].Effort,
		cfgPath:      cfgPath,
		isTTY:        tty.IsTerminal(os.Stdout),
		env:          envcontext.Detect(context.Background(), envcontext.Options{ShellHistory: config.ResolveBoolSetting(cfg, "shell_context")}),
	}, nil
}
