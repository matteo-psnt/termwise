# termwise

A fast terminal assistant for command generation, quick answers, and lightweight agent work. Describe what you want, get the command or the answer.

Binary: `termwise` | Alias: `tw`

## The Problem

Full AI agents (Claude Code, Cursor, etc.) are powerful but heavy. Spinning one up just to remember a `find` flag or ask a quick question about your project is overkill. There's a gap between "google it" and "start an agent session."

## How It Works

```bash
# Start the TUI with an initial prompt already sent
tw "undo last commit but keep changes"

# Run the headless agent path
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
- **Terminal-native** — TUI command responses can be pushed into your shell buffer; headless runs print to stdout
- **Minimal** — open the TUI when you need context, use `tw ask` when you just want the final response
- **Zero friction** — single binary, brew installable, works with existing API keys
