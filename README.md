# termwise

A terminal assistant for command generation and quick answers. Describe what you want, get the command or the answer — without leaving the shell.

Binary: `termwise` | Alias: `tw`

## The Problem

Full AI agents (Claude Code, Cursor, etc.) are powerful but heavy. Spinning one up just to remember a `find` flag or ask a quick question about your project is overkill. There's a gap between "google it" and "start an agent session."

## How It Works

```bash
# Open the TUI with an initial prompt already sent
tw "undo last commit but keep changes"

# Headless — prints only the final response
tw ask "undo last commit but keep changes"
# → git reset --soft HEAD~1

tw ask "what port is my dev server using"

# Ctrl+T in your shell — open the TUI with the current buffer prefilled
$ restart the docker containers<Ctrl+T>
```

Open the TUI for back-and-forth when you need to figure something out:

```bash
tw
# Interactive agent with file reading, command execution, and conversation
```

Every entry point runs the same agent loop. It reads files, searches code, and runs read-only commands to check facts before answering. Commands that change anything are **proposed, not executed** — you review them, optionally edit them, and push them into your shell buffer.

## Principles

- **Terminal-native** — TUI command responses can be pushed into your shell buffer; headless runs print to stdout
- **Verified, not guessed** — the agent checks what's installed and where a binary came from before proposing a command
- **Read-only by default** — it never edits your files; `bash` is limited to read-only commands and temp-directory scratch, and anything else needs your approval
- **Minimal** — open the TUI when you need context, use `tw ask` when you just want the final response
- **Zero friction** — single binary, brew installable, works with existing API keys

## What It Knows About Your Machine

The agent is given your OS, shell, working directory, detected project type, git branch and state, and — importantly — which package managers and CLI tools you actually have installed. This is what stops it proposing `npm uninstall` for a Homebrew-installed binary, or a `rg` command on a machine without ripgrep.

Recent shell history is **off by default** because it can contain secrets. Turn it on with:

```bash
tw config    # → Shell context
```

## Logs

Local only, under `~/.config/termwise/`:

| File | Contents |
|---|---|
| `prompt-history.jsonl` | Prompts you've typed, for up-arrow recall |
| `exchanges.jsonl` | Completed turns — prompt, response, tools used, model |
| `sessions/` | Full conversations, for `--resume` |

## Building

```bash
go install ./cmd/termwise/...
```
