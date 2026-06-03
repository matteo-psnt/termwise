# termwise

A fast terminal assistant for command generation and quick answers. Describe what you want, get the command or the answer.

Binary: `termwise` | Alias: `tw`

## The Problem

Full AI agents (Claude Code, Cursor, etc.) are powerful but heavy. Spinning one up just to remember a `find` flag or ask a quick question about your project is overkill. There's a gap between "google it" and "start an agent session."

## How It Works

```bash
# Start the TUI with an initial prompt already sent
tw "undo last commit but keep changes"

# Run the headless one-shot path
tw ask "undo last commit but keep changes"
# → git reset --soft HEAD~1

# Ask a quick question without opening the TUI
tw ask "what port is my dev server using"

# Ctrl+T in your shell — open the TUI with the current buffer prefilled
$ restart the docker containers<Ctrl+T>
```

Open the TUI for back-and-forth when you need to figure something out:

```bash
tw
# Interactive agent with file reading, command execution, and conversation
```

## Principles

- **Fast** — sub-second responses, small model, minimal prompt
- **Terminal-native** — commands land in your shell buffer, ready to run
- **Minimal** — no TUI for one-shot runs, no project scaffolding
- **Zero friction** — single binary, brew installable, works with existing API keys
