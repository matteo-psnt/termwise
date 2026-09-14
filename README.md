# termwise

A terminal assistant for command generation and quick answers. Describe what you want, get the command or the answer — without leaving the shell.

Binary: `termwise` | Alias: `tw`

## The Problem

Full AI agents (Claude Code, Cursor, etc.) are powerful but heavy. Spinning one up just to remember a `find` flag or ask a quick question about your project is overkill. There's a gap between "google it" and "start an agent session."

## How It Works

Describe what you want. It checks before it answers, then proposes a command for
you to review — it never runs it for you.

![The interactive loop](assets/agent.gif)

### Ctrl+T — from your shell prompt

Type the intent where you would have typed the command, press **Ctrl+T**, and
the finished command comes back in your buffer, ready to edit or run.

![The shell widget](assets/widget.gif)

### `tw ask` — headless

One shot, straight to stdout, no TUI. Good for piping and for quick lookups.

![Headless ask](assets/ask.gif)

### `@` — point at a file

Type `@` in the prompt and Tab through your project. Directories complete with
a trailing `/` and reopen on their contents, so descending a tree is one
keystroke per level; dotfiles stay out of the way unless you type the leading
dot yourself.

```
› explain @internal/agent/tools/bash.go
```

The `@` survives into the prompt — the agent is told that an `@`-prefixed path
is a file you are pointing it at, so it reads the file before answering. That
works in `tw ask` and `tw explain` too, where there is no dropdown to help you
type it.

### Settings live in the session

`/theme`, `/model`, `/effort` and `/config` apply as you move through them.

![Theme picker](assets/config.gif)

```bash
tw                      # open the TUI
tw "undo my last commit but keep the changes"
tw ask "how do I run the dev server here"
tw explain 'awk -F: "{print $1}" /etc/passwd'
```

### `tw explain` — the other direction

Every other entry point turns intent into a command. `tw explain` goes the
other way: give it a command and it breaks it down flag by flag, checking `man`
and `--help` rather than guessing, and calling out anything destructive. Run it
bare to get an input box, or pipe a command in:

```bash
tw explain              # asks which command to explain
fc -ln -1 | tw explain  # explains the command you just ran
```

It is given no `command` tool at all, so it can only explain — it will never
hand you something new to run.

Every entry point runs the same agent loop. It reads files, searches code, and
runs read-only commands to check facts before answering. Commands that change
anything are **proposed, not executed** — you review them, optionally edit them,
and push them into your shell buffer.

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
