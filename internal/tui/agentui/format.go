package agentui

import (
	"errors"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// providerErrMsg returns the error string with contextual advice appended for
// known recoverable error kinds (auth failure, model not found).
func providerErrMsg(err error) string {
	if pe, ok := errors.AsType[*provider.Error](err); ok {
		switch pe.Kind {
		case provider.ErrAuth:
			return pe.Reason + " Run `tw config` to update your key."
		case provider.ErrModelNotFound:
			return pe.Reason + " Run `tw config` to change the model."
		}
	}
	return err.Error()
}

// toolDetail returns the display string for a tool call (command or path).
func toolDetail(tc provider.ToolCall) string {
	switch tc.Name {
	case "bash":
		cmd, _ := tc.Input["command"].(string)
		return cmd
	case "read":
		path, _ := tc.Input["path"].(string)
		return path
	case "grep":
		pattern, _ := tc.Input["pattern"].(string)
		if path, _ := tc.Input["path"].(string); path != "" {
			return pattern + " in " + path
		}
		return pattern
	case "glob":
		pattern, _ := tc.Input["pattern"].(string)
		if path, _ := tc.Input["path"].(string); path != "" {
			return pattern + " in " + path
		}
		return pattern
	default:
		return ""
	}
}
