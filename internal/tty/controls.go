package tty

import "strings"

// VisibleControls makes control characters in text from outside termwise
// (model output, tool output, a proposed command, a resumed session) visible
// instead of acting on the terminal. Printed raw, an ESC can recolour or
// clear the screen, move the cursor over earlier output, retitle the window
// or write the clipboard (OSC 52), and a lone CR overwrites the line it is on.
// Each such character is shown as its Unicode control picture (␛, ␍, …).
// Newlines and tabs are layout and are kept; CRLF line endings become LF.
func VisibleControls(s string) string {
	if !strings.ContainsFunc(s, isHiddenControl) {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case !isHiddenControl(r):
			return r
		case r < 0x20:
			return 0x2400 + r // ␀ … ␟
		case r == 0x7f:
			return '␡'
		default: // C1 controls, U+0080–U+009F
			return '�'
		}
	}, s)
}

func isHiddenControl(r rune) bool {
	return (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
