# termwise

A fast terminal assistant for command generation and quick answers. Describe what you want, get the command or the answer.

Binary: `termwise` | Alias: `tw`

## The Problem

Full AI agents (Claude Code, Cursor, etc.) are powerful but heavy. Spinning one up just to remember a `find` flag or ask a quick question about your project is overkill. There's a gap between "google it" and "start an agent session."

## How It Works

```bash
# Describe what you want → get the command, ready to run
tw "undo last commit but keep changes"
# → git reset --soft HEAD~1  (lands in your shell buffer, hit enter to run)

# Ask a quick question
tw "what port is my dev server using"

# Ctrl+T in your shell — type a description, get the command inline
$ restart the docker containers<Ctrl+T>
$ docker compose restart
```

Open the TUI for back-and-forth when you need to figure something out:

```bash
tw
# Interactive agent with file reading, command execution, and conversation
```

## Principles

- **Fast** — sub-second responses, small model, minimal prompt
- **Terminal-native** — commands land in your shell buffer, ready to run
- **Minimal** — no conversation in single-shot, no project scaffolding
- **Zero friction** — single binary, brew installable, works with existing API keys

## Status

Planning complete. See `docs/` for design notes.
