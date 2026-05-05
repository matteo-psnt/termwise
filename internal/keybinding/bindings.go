package keybinding

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Label returns a human-readable label for a stored zsh bindkey string.
// e.g. "^T" → "Ctrl+T", "\e[15~" → "F5", "\ef" → "Alt+F".
// Falls back to the raw string if unrecognised.
func Label(s string) string {
	if label, ok := zshLabelMap[s]; ok {
		return label
	}
	// ^X → Ctrl+X
	if len(s) == 2 && s[0] == '^' && s[1] >= 'A' && s[1] <= 'Z' {
		return "Ctrl+" + string(rune(s[1]))
	}
	// \eX → Alt+X (single printable char, not a sequence prefix like [ or O)
	if strings.HasPrefix(s, `\e`) && len(s) == 3 && s[2] != '[' && s[2] != 'O' {
		return "Alt+" + strings.ToUpper(string(rune(s[2])))
	}
	// \e^X → Alt+Ctrl+X
	if strings.HasPrefix(s, `\e^`) && len(s) == 4 && s[3] >= 'A' && s[3] <= 'Z' {
		return "Alt+Ctrl+" + string(rune(s[3]))
	}
	return s
}

// ToBash converts ^X notation to \C-x for bash's bind -x.
func ToBash(kb string) string {
	if len(kb) == 2 && kb[0] == '^' {
		return fmt.Sprintf(`\C-%c`, kb[1]+'a'-'A')
	}
	return kb
}

// zshLabelMap is built at init from keySeqTable + keyTypeLabel.
var zshLabelMap map[string]string

func init() {
	type key struct {
		t   tea.KeyType
		alt bool
	}
	zshLabelMap = make(map[string]string, len(keySeqTable)+8)
	for k, seq := range keySeqTable {
		base, ok := keyTypeLabel[k.t]
		if !ok {
			continue
		}
		label := base
		if k.alt {
			label = "Alt+" + base
		}
		zshLabelMap[seq] = label
	}
	// Ctrl-range special characters not covered by the ^X pattern.
	zshLabelMap["^@"] = "Ctrl+@"
	zshLabelMap[`^\`] = `Ctrl+\`
	zshLabelMap["^]"] = "Ctrl+]"
	zshLabelMap["^^"] = "Ctrl+^"
	zshLabelMap["^_"] = "Ctrl+_"
	zshLabelMap["^?"] = "Ctrl+?"
}

// keyTypeLabel maps a KeyType to its base human label (no modifier prefix).
var keyTypeLabel = map[tea.KeyType]string{
	// Arrows
	tea.KeyUp:    "Up",
	tea.KeyDown:  "Down",
	tea.KeyRight: "Right",
	tea.KeyLeft:  "Left",

	tea.KeyShiftUp:    "Shift+Up",
	tea.KeyShiftDown:  "Shift+Down",
	tea.KeyShiftRight: "Shift+Right",
	tea.KeyShiftLeft:  "Shift+Left",

	tea.KeyCtrlUp:    "Ctrl+Up",
	tea.KeyCtrlDown:  "Ctrl+Down",
	tea.KeyCtrlRight: "Ctrl+Right",
	tea.KeyCtrlLeft:  "Ctrl+Left",

	tea.KeyCtrlShiftUp:    "Ctrl+Shift+Up",
	tea.KeyCtrlShiftDown:  "Ctrl+Shift+Down",
	tea.KeyCtrlShiftRight: "Ctrl+Shift+Right",
	tea.KeyCtrlShiftLeft:  "Ctrl+Shift+Left",

	// Home / End
	tea.KeyHome:          "Home",
	tea.KeyEnd:           "End",
	tea.KeyCtrlHome:      "Ctrl+Home",
	tea.KeyCtrlEnd:       "Ctrl+End",
	tea.KeyShiftHome:     "Shift+Home",
	tea.KeyShiftEnd:      "Shift+End",
	tea.KeyCtrlShiftHome: "Ctrl+Shift+Home",
	tea.KeyCtrlShiftEnd:  "Ctrl+Shift+End",

	// Page
	tea.KeyPgUp:       "PgUp",
	tea.KeyPgDown:     "PgDown",
	tea.KeyCtrlPgUp:   "Ctrl+PgUp",
	tea.KeyCtrlPgDown: "Ctrl+PgDown",

	// Insert / Delete
	tea.KeyInsert: "Insert",
	tea.KeyDelete: "Delete",

	// Tab
	tea.KeyShiftTab: "Shift+Tab",

	// Function keys
	tea.KeyF1:  "F1",
	tea.KeyF2:  "F2",
	tea.KeyF3:  "F3",
	tea.KeyF4:  "F4",
	tea.KeyF5:  "F5",
	tea.KeyF6:  "F6",
	tea.KeyF7:  "F7",
	tea.KeyF8:  "F8",
	tea.KeyF9:  "F9",
	tea.KeyF10: "F10",
	tea.KeyF11: "F11",
	tea.KeyF12: "F12",
	tea.KeyF13: "F13",
	tea.KeyF14: "F14",
	tea.KeyF15: "F15",
	tea.KeyF16: "F16",
	tea.KeyF17: "F17",
	tea.KeyF18: "F18",
	tea.KeyF19: "F19",
	tea.KeyF20: "F20",
}

// KeyMsgToZsh converts a bubbletea key event to a zsh bindkey-compatible string.
// Returns ("", false) if the key cannot be used as a terminal keybinding.
func KeyMsgToZsh(msg tea.KeyMsg) (string, bool) {
	// Alt+rune (e.g. alt+f): \ef
	if msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		return `\e` + string(msg.Runes), true
	}

	// Ctrl+A through Ctrl+Z → ^A through ^Z (type values 1–26).
	if msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ {
		letter := byte('A' + int(msg.Type) - int(tea.KeyCtrlA))
		if msg.Alt {
			return `\e^` + string([]byte{letter}), true
		}
		return "^" + string([]byte{letter}), true
	}

	// Other ctrl-range keys not in 1–26.
	switch msg.Type {
	case tea.KeyCtrlAt:
		return "^@", true
	case tea.KeyCtrlBackslash:
		return `^\`, true
	case tea.KeyCtrlCloseBracket:
		return "^]", true
	case tea.KeyCtrlCaret:
		return "^^", true
	case tea.KeyCtrlUnderscore:
		return "^_", true
	case tea.KeyCtrlQuestionMark:
		return "^?", true
	}

	// Special / escape-sequence keys: look up canonical xterm sequence.
	type key struct {
		t   tea.KeyType
		alt bool
	}
	seq, ok := keySeqTable[key{msg.Type, msg.Alt}]
	return seq, ok
}

// keySeqTable maps (KeyType, alt) → canonical zsh bindkey string.
// Sequences use \e for ESC, matching what zsh bindkey expects.
var keySeqTable = map[struct {
	t   tea.KeyType
	alt bool
}]string{
	// ── Arrow keys ──────────────────────────────────────────────────────────
	{tea.KeyUp, false}:    `\e[A`,
	{tea.KeyDown, false}:  `\e[B`,
	{tea.KeyRight, false}: `\e[C`,
	{tea.KeyLeft, false}:  `\e[D`,

	{tea.KeyUp, true}:    `\e[1;3A`,
	{tea.KeyDown, true}:  `\e[1;3B`,
	{tea.KeyRight, true}: `\e[1;3C`,
	{tea.KeyLeft, true}:  `\e[1;3D`,

	{tea.KeyShiftUp, false}:    `\e[1;2A`,
	{tea.KeyShiftDown, false}:  `\e[1;2B`,
	{tea.KeyShiftRight, false}: `\e[1;2C`,
	{tea.KeyShiftLeft, false}:  `\e[1;2D`,

	{tea.KeyCtrlUp, false}:    `\e[1;5A`,
	{tea.KeyCtrlDown, false}:  `\e[1;5B`,
	{tea.KeyCtrlRight, false}: `\e[1;5C`,
	{tea.KeyCtrlLeft, false}:  `\e[1;5D`,

	{tea.KeyCtrlUp, true}:    `\e[1;7A`,
	{tea.KeyCtrlDown, true}:  `\e[1;7B`,
	{tea.KeyCtrlRight, true}: `\e[1;7C`,
	{tea.KeyCtrlLeft, true}:  `\e[1;7D`,

	{tea.KeyCtrlShiftUp, false}:    `\e[1;6A`,
	{tea.KeyCtrlShiftDown, false}:  `\e[1;6B`,
	{tea.KeyCtrlShiftRight, false}: `\e[1;6C`,
	{tea.KeyCtrlShiftLeft, false}:  `\e[1;6D`,

	{tea.KeyCtrlShiftUp, true}:    `\e[1;8A`,
	{tea.KeyCtrlShiftDown, true}:  `\e[1;8B`,
	{tea.KeyCtrlShiftRight, true}: `\e[1;8C`,
	{tea.KeyCtrlShiftLeft, true}:  `\e[1;8D`,

	{tea.KeyShiftUp, true}:    `\e[1;4A`,
	{tea.KeyShiftDown, true}:  `\e[1;4B`,
	{tea.KeyShiftRight, true}: `\e[1;4C`,
	{tea.KeyShiftLeft, true}:  `\e[1;4D`,

	// ── Home / End ──────────────────────────────────────────────────────────
	{tea.KeyHome, false}: `\e[H`,
	{tea.KeyEnd, false}:  `\e[F`,
	{tea.KeyHome, true}:  `\e[1;3H`,
	{tea.KeyEnd, true}:   `\e[1;3F`,

	{tea.KeyCtrlHome, false}: `\e[1;5H`,
	{tea.KeyCtrlEnd, false}:  `\e[1;5F`,
	{tea.KeyCtrlHome, true}:  `\e[1;7H`,
	{tea.KeyCtrlEnd, true}:   `\e[1;7F`,

	{tea.KeyShiftHome, false}: `\e[1;2H`,
	{tea.KeyShiftEnd, false}:  `\e[1;2F`,
	{tea.KeyShiftHome, true}:  `\e[1;4H`,
	{tea.KeyShiftEnd, true}:   `\e[1;4F`,

	{tea.KeyCtrlShiftHome, false}: `\e[1;6H`,
	{tea.KeyCtrlShiftEnd, false}:  `\e[1;6F`,
	{tea.KeyCtrlShiftHome, true}:  `\e[1;8H`,
	{tea.KeyCtrlShiftEnd, true}:   `\e[1;8F`,

	// ── Page Up / Down ──────────────────────────────────────────────────────
	{tea.KeyPgUp, false}:   `\e[5~`,
	{tea.KeyPgDown, false}: `\e[6~`,
	{tea.KeyPgUp, true}:    `\e[5;3~`,
	{tea.KeyPgDown, true}:  `\e[6;3~`,

	{tea.KeyCtrlPgUp, false}:   `\e[5;5~`,
	{tea.KeyCtrlPgDown, false}: `\e[6;5~`,
	{tea.KeyCtrlPgUp, true}:    `\e[5;7~`,
	{tea.KeyCtrlPgDown, true}:  `\e[6;7~`,

	// ── Insert / Delete ─────────────────────────────────────────────────────
	{tea.KeyInsert, false}: `\e[2~`,
	{tea.KeyDelete, false}: `\e[3~`,
	{tea.KeyInsert, true}:  `\e[2;3~`,
	{tea.KeyDelete, true}:  `\e[3;3~`,

	// ── Shift+Tab ───────────────────────────────────────────────────────────
	{tea.KeyShiftTab, false}: `\e[Z`,

	// ── Function keys (xterm/vt100 canonical sequences) ─────────────────────
	{tea.KeyF1, false}:  `\eOP`,
	{tea.KeyF2, false}:  `\eOQ`,
	{tea.KeyF3, false}:  `\eOR`,
	{tea.KeyF4, false}:  `\eOS`,
	{tea.KeyF5, false}:  `\e[15~`,
	{tea.KeyF6, false}:  `\e[17~`,
	{tea.KeyF7, false}:  `\e[18~`,
	{tea.KeyF8, false}:  `\e[19~`,
	{tea.KeyF9, false}:  `\e[20~`,
	{tea.KeyF10, false}: `\e[21~`,
	{tea.KeyF11, false}: `\e[23~`,
	{tea.KeyF12, false}: `\e[24~`,
	{tea.KeyF13, false}: `\e[1;2P`,
	{tea.KeyF14, false}: `\e[1;2Q`,
	{tea.KeyF15, false}: `\e[1;2R`,
	{tea.KeyF16, false}: `\e[1;2S`,
	{tea.KeyF17, false}: `\e[15;2~`,
	{tea.KeyF18, false}: `\e[17;2~`,
	{tea.KeyF19, false}: `\e[18;2~`,
	{tea.KeyF20, false}: `\e[19;2~`,

	{tea.KeyF1, true}:  `\e[1;3P`,
	{tea.KeyF2, true}:  `\e[1;3Q`,
	{tea.KeyF3, true}:  `\e[1;3R`,
	{tea.KeyF4, true}:  `\e[1;3S`,
	{tea.KeyF5, true}:  `\e[15;3~`,
	{tea.KeyF6, true}:  `\e[17;3~`,
	{tea.KeyF7, true}:  `\e[18;3~`,
	{tea.KeyF8, true}:  `\e[19;3~`,
	{tea.KeyF9, true}:  `\e[20;3~`,
	{tea.KeyF10, true}: `\e[21;3~`,
	{tea.KeyF11, true}: `\e[23;3~`,
	{tea.KeyF12, true}: `\e[24;3~`,
	{tea.KeyF13, true}: `\e[25;3~`,
	{tea.KeyF14, true}: `\e[26;3~`,
	{tea.KeyF15, true}: `\e[28;3~`,
	{tea.KeyF16, true}: `\e[29;3~`,
}
