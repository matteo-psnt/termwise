package runner

import (
	"context"
	"path/filepath"
	"time"

	"github.com/matteo-psnt/termwise/internal/agent"
	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
)

const judgeTimeout = 10 * time.Second

// Ask runs the non-interactive headless agent path used by `tw ask`.
func Ask(ctx context.Context, prompt string) error {
	var err error
	prompt, err = appendPromptStdin(prompt)
	if err != nil {
		return err
	}

	rt, err := resolveRuntime()
	if err != nil {
		return err
	}

	output, err := agent.RunHeadless(ctx, newAskHeadlessConfig(rt, prompt))
	logAskExchange(rt, prompt, output, err)
	if err != nil {
		return err
	}
	return writeFinalOutput(output, rt.isTTY)
}

func newAskHeadlessConfig(rt runtimeContext, prompt string) agent.HeadlessConfig {
	return agent.HeadlessConfig{
		Client:        rt.client,
		Model:         rt.modelID,
		System:        systemprompt.AgentHeadless(agenttools.HeadlessDefs, rt.env),
		Prompt:        prompt,
		Tools:         agenttools.HeadlessDefs,
		Effort:        config.EffectiveEffort(rt.providerName, rt.modelID, rt.providerCfg.Effort),
		NeedsApproval: allowlist.NeedsApproval,
		OnNeedsApproval: func(ctx context.Context, step agent.ToolStep) provider.ToolResult {
			return headlessApprovalResult(ctx, rt, step)
		},
		OnAsk: unavailableAskToolResult,
	}
}

func unavailableAskToolResult(step agent.ToolStep) provider.ToolResult {
	return provider.ToolResult{
		ToolCallID: step.ToolCall.ID,
		Content:    "error: interactive ask tool is unavailable in `tw ask`; respond directly instead",
		IsError:    true,
	}
}

func headlessApprovalResult(ctx context.Context, rt runtimeContext, step agent.ToolStep) provider.ToolResult {
	if !rt.llmJudge {
		return provider.ToolResult{
			ToolCallID: step.ToolCall.ID,
			Content:    "error: command requires interactive approval and `tw ask` cannot run it because llm_judge is disabled",
			IsError:    true,
		}
	}

	judgeCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()

	safe, err := agent.JudgeBashCommand(judgeCtx, rt.client, rt.modelID, step.Command)
	if err != nil {
		return provider.ToolResult{
			ToolCallID: step.ToolCall.ID,
			Content:    "error: command requires approval and llm_judge failed to auto-approve it in `tw ask`",
			IsError:    true,
		}
	}
	if !safe {
		return provider.ToolResult{
			ToolCallID: step.ToolCall.ID,
			Content:    "error: command was not auto-approved by llm_judge in `tw ask`",
			IsError:    true,
		}
	}
	return agenttools.ExecuteBash(ctx, step.ToolCall)
}

// logAskExchange records the turn so headless output can be reviewed later,
// the same way the TUI records its own. Best-effort: never blocks the result.
func logAskExchange(rt runtimeContext, prompt string, output agent.FinalResponse, runErr error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return
	}
	e := history.Exchange{
		Prompt:   prompt,
		Kind:     history.ExchangeKindText,
		Response: output.Content,
		Provider: rt.providerName,
		Model:    rt.modelID,
	}
	if runErr != nil {
		e.Kind = history.ExchangeKindError
		e.Response = runErr.Error()
	} else if output.Type == "command" {
		e.Kind = history.ExchangeKindCommand
	}
	_ = history.NewExchangeLog(filepath.Dir(cfgPath)).Append(e)
}
