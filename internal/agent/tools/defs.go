package tools

import "github.com/matteo-psnt/termwise/internal/provider"

// Defs are the tool definitions sent to the model in the interactive agent TUI.
var Defs = []provider.ToolDef{ReadDef, BashDef, GrepDef, GlobDef, AskDef, CommandDef}

// HeadlessDefs are the tool definitions available to non-interactive agent runs.
var HeadlessDefs = []provider.ToolDef{ReadDef, BashDef, GrepDef, GlobDef, CommandDef}
