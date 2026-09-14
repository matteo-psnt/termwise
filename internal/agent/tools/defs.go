package tools

import "github.com/matteo-psnt/termwise/internal/provider"

// Defs are the tool definitions sent to the model in the interactive agent TUI.
var Defs = []provider.ToolDef{ReadDef, BashDef, GrepDef, GlobDef, AskDef, CommandDef}

// HeadlessDefs are the tool definitions available to non-interactive agent runs.
var HeadlessDefs = []provider.ToolDef{ReadDef, BashDef, GrepDef, GlobDef, CommandDef}

// ExplainDefs are the tool definitions for `tw explain`. CommandDef is absent
// because explaining a command the user already has never requires proposing a
// new one; bash stays so the model can check `man` or `--help` before it
// commits to what a flag does.
var ExplainDefs = []provider.ToolDef{ReadDef, BashDef, GrepDef, GlobDef, AskDef}
