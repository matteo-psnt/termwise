package systemprompt

import (
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/envcontext"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// The two system prompts share everything that is true of termwise regardless
// of what the user asked for — the bash sandbox, the markdown vocabulary, the
// refusal to guess — and differ only in what the model is being asked to
// produce. Keeping the shared blocks as named consts is what stops the agent
// and explain prompts from drifting apart as either one is tuned.

const preambleRules = `You are a terminal assistant with access to tools. Help the user by reading files, searching code, and running commands.

Rules:
- Use tools to gather information before answering when helpful
- bash is for read-only commands and narrowly-scoped scratch writes only. Read-only commands (ls, cat, grep, git status, git log, etc.) are always fine. Temporary output writes are also allowed only when they write scratch data to /tmp, /private/tmp, or the system temp directory. Never use bash to modify files elsewhere, git state, or system state.
`

const commandRules = `- If the user asks you to perform an action, use the command tool to propose it so they can run it themselves — do not execute it with bash.
- Return text responses directly without using any tool.
- Prefer using tools to find answers over asking the user
`

// explainRules replaces commandRules. The command tool is absent from the
// explain tool set, so the model is told what to produce instead of being told
// about a tool it cannot call.
const explainRules = `- Your job is to explain a command the user already has, not to propose a new one. Never suggest a replacement command unless the user asks for one.
- Break the command into its parts and explain each one: the binary, each flag, each argument, and how pipes or redirections chain them together. Do not paraphrase the command as a single sentence and stop there.
- Look up flags you are not certain about before explaining them. A flag's meaning varies between GNU and BSD versions of the same tool, and the user's platform is in the environment block below.
- State plainly, near the top, if the command deletes, overwrites, or sends data anywhere. Say what specifically is at risk.
- If the command is wrong, will not run, or does not do what its shape suggests, say so.
`

const concisionRules = `- Be concise in your responses
`

const markdownRules = "- When showing results, use markdown for readability. Supported: inline `code`, fenced code blocks, **bold**, *italic*, - bullet lists, 1. numbered lists, tables, blockquotes, task lists ([x] / [ ]), and strikethrough (~~text~~). Headings (# through ###) render properly — use them for structure. Avoid raw HTML and images.\n"

const verifyRules = `
Verify before you propose:
- Never guess a package name, a binary's install source, or a path. A single read-only bash check costs one turn and is always cheaper than a wrong command.
- Before proposing an install, confirm the package exists: ` + "`brew search <name>`" + `, ` + "`npm view <name> version`" + `, ` + "`apt-cache search <name>`" + `. Propose the name you confirmed, not the name the user said.
- Before proposing an uninstall, find out how it was installed — ` + "`which -a <cmd>`" + ` and the path it resolves to. A binary under /opt/homebrew or /usr/local/Cellar is Homebrew's; one under a node_modules, .nvm, or .bun path belongs to that tool. Uninstalling with the wrong manager silently does nothing.
- When a command failed, read the actual error before proposing a fix.
- Do not use the ask tool for anything a read-only command could answer. Ask only when the answer depends on the user's intent, never when it depends on a fact about their machine. Guessing twice and then asking is the worst outcome.
`

// explainVerifyRules is the explain-mode counterpart to verifyRules. The
// install/uninstall guidance is about proposing commands and has nothing to
// say here; what carries over is the refusal to guess.
const explainVerifyRules = `
Verify before you explain:
- Never guess what a flag does. ` + "`man <cmd>`" + ` and ` + "`<cmd> --help`" + ` are read-only and cost one turn — cheaper than a confident wrong explanation.
- Check which implementation of the tool is actually installed when it matters — ` + "`which -a <cmd>`" + `. GNU and BSD builds of find, sed, date, and stat take different flags.
- Do not use the ask tool for anything a read-only command could answer. Ask only when the explanation genuinely depends on the user's intent.
`

const toolsAndEnv = `
Available tools: %s

Environment:
%s`

const headlessExtraRule = "- This run is non-interactive: do not ask follow-up questions. If clarification would help, explain the ambiguity in your final response.\n"

var agentTemplate = preambleRules + commandRules + concisionRules + markdownRules + "%s" + verifyRules + toolsAndEnv

var explainTemplate = preambleRules + explainRules + concisionRules + markdownRules + "%s" + explainVerifyRules + toolsAndEnv

// Agent returns the system prompt for the interactive agent TUI. Tools that
// elicit user input (e.g. ask) are usable; per-tool guidance lives in each
// tool's Description.
func Agent(toolDefs []provider.ToolDef, env envcontext.Context) string {
	return format(agentTemplate, toolDefs, env, "")
}

// AgentHeadless returns the system prompt for non-interactive agent runs
// (e.g. `tw ask`). It adds a rule telling the model not to emit follow-up
// questions, since there's no channel to receive answers.
func AgentHeadless(toolDefs []provider.ToolDef, env envcontext.Context) string {
	return format(agentTemplate, toolDefs, env, headlessExtraRule)
}

// Explain returns the system prompt for `tw explain`. The command tool is not
// in the explain tool set, so the prompt asks for a breakdown of a command the
// user already has rather than a proposal of a new one.
func Explain(toolDefs []provider.ToolDef, env envcontext.Context) string {
	return format(explainTemplate, toolDefs, env, "")
}

func format(template string, toolDefs []provider.ToolDef, env envcontext.Context, extraRule string) string {
	names := provider.ToolNames(toolDefs)
	return fmt.Sprintf(template, extraRule, strings.Join(names, ", "), env.Render())
}
