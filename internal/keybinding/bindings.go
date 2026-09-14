package keybinding

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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

// ToFish converts a stored zsh bindkey string to a fish 4 key name, e.g.
// "^T" → "ctrl-t", "\\ef" → "alt-f", "\\e[15~" → "f5".
//
// fish 4 dropped the \\ck escape form: `bind '\\ck'` there binds the three-key
// sequence backslash, c, k rather than Ctrl+K, so only the named form works.
// Label already resolves every sequence termwise can store into "Ctrl+T" /
// "Alt+F" / "F5" shape, which is the fish name modulo case and separator.
//
// Returns ok=false when the binding has no fish equivalent — Label echoes back
// anything it does not recognise, and echoing that at fish would bind garbage.
func ToFish(kb string) (string, bool) {
	label := Label(kb)
	if label == kb && !strings.HasPrefix(kb, "^") {
		return "", false
	}
	name := strings.ToLower(strings.ReplaceAll(label, "+", "-"))
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isDigit && r != '-' {
			return "", false
		}
	}
	return name, true
}

// zshLabelMap is built at init from keySeqTable + codeLabel.
var zshLabelMap map[string]string

// key identifies a non-printable key by its v2 key code and exact modifier set.
type key struct {
	code rune
	mod  tea.KeyMod
}

func init() {
	zshLabelMap = make(map[string]string, len(keySeqTable)+8)
	for k, seq := range keySeqTable {
		base, ok := codeLabel[k.code]
		if !ok {
			continue
		}
		zshLabelMap[seq] = modLabel(k.mod) + base
	}
	// Ctrl-range special characters not covered by the ^X pattern.
	zshLabelMap["^@"] = "Ctrl+@"
	zshLabelMap[`^\`] = `Ctrl+\`
	zshLabelMap["^]"] = "Ctrl+]"
	zshLabelMap["^^"] = "Ctrl+^"
	zshLabelMap["^_"] = "Ctrl+_"
	zshLabelMap["^?"] = "Ctrl+?"
}

// modLabel renders a modifier prefix in the canonical Ctrl+Alt+Shift order.
func modLabel(mod tea.KeyMod) string {
	var s string
	if mod.Contains(tea.ModCtrl) {
		s += "Ctrl+"
	}
	if mod.Contains(tea.ModAlt) {
		s += "Alt+"
	}
	if mod.Contains(tea.ModShift) {
		s += "Shift+"
	}
	return s
}

// codeLabel maps a special key code to its base human label (no modifiers).
var codeLabel = map[rune]string{
	tea.KeyUp:     "Up",
	tea.KeyDown:   "Down",
	tea.KeyRight:  "Right",
	tea.KeyLeft:   "Left",
	tea.KeyHome:   "Home",
	tea.KeyEnd:    "End",
	tea.KeyPgUp:   "PgUp",
	tea.KeyPgDown: "PgDown",
	tea.KeyInsert: "Insert",
	tea.KeyDelete: "Delete",
	tea.KeyTab:    "Tab",
	tea.KeyF1:     "F1",
	tea.KeyF2:     "F2",
	tea.KeyF3:     "F3",
	tea.KeyF4:     "F4",
	tea.KeyF5:     "F5",
	tea.KeyF6:     "F6",
	tea.KeyF7:     "F7",
	tea.KeyF8:     "F8",
	tea.KeyF9:     "F9",
	tea.KeyF10:    "F10",
	tea.KeyF11:    "F11",
	tea.KeyF12:    "F12",
	tea.KeyF13:    "F13",
	tea.KeyF14:    "F14",
	tea.KeyF15:    "F15",
	tea.KeyF16:    "F16",
	tea.KeyF17:    "F17",
	tea.KeyF18:    "F18",
	tea.KeyF19:    "F19",
	tea.KeyF20:    "F20",
}

// ctrlCaret returns the caret-notation character for a Ctrl-combinable code,
// e.g. 'a' → "A", '\\' → "\\". The bool is false for codes that have no
// ^X form.
func ctrlCaret(code rune) (string, bool) {
	switch {
	case code >= 'a' && code <= 'z':
		return string('A' + (code - 'a')), true
	case code >= 'A' && code <= 'Z':
		return string(code), true
	}
	switch code {
	case '@', '\\', ']', '^', '_', '?':
		return string(code), true
	case tea.KeySpace: // Ctrl+Space → ^@ (NUL)
		return "@", true
	}
	return "", false
}

// KeyMsgToZsh converts a bubbletea key event to a zsh bindkey-compatible string.
// Returns ("", false) if the key cannot be used as a terminal keybinding.
func KeyMsgToZsh(k tea.Key) (string, bool) {
	mod := k.Mod
	alt := mod.Contains(tea.ModAlt)

	// Ctrl[+Alt]+char → ^X / \e^X. Covers Ctrl+A–Z and the ctrl-range
	// punctuation (^@ ^\ ^] ^^ ^_ ^?). Shift is excluded so combos like
	// Ctrl+Shift+Up fall through to the sequence table.
	if mod.Contains(tea.ModCtrl) && !mod.Contains(tea.ModShift) {
		if c, ok := ctrlCaret(k.Code); ok {
			if alt {
				return `\e^` + c, true
			}
			return "^" + c, true
		}
	}

	// Alt+printable rune (no other modifiers) → \eX.
	if mod == tea.ModAlt && k.Code >= '!' && k.Code <= '~' {
		return `\e` + string(k.Code), true
	}

	// Special / escape-sequence keys: look up the canonical xterm sequence.
	seq, ok := keySeqTable[key{k.Code, mod}]
	return seq, ok
}

// keySeqTable maps (code, modifiers) → canonical zsh bindkey string.
// Sequences use \e for ESC, matching what zsh bindkey expects. The modifier
// parameter follows xterm's encoding: 1 + shift(1) + alt(2) + ctrl(4).
var keySeqTable = map[key]string{
	// ── Arrow keys ──────────────────────────────────────────────────────────
	{tea.KeyUp, 0}:    `\e[A`,
	{tea.KeyDown, 0}:  `\e[B`,
	{tea.KeyRight, 0}: `\e[C`,
	{tea.KeyLeft, 0}:  `\e[D`,

	{tea.KeyUp, tea.ModAlt}:    `\e[1;3A`,
	{tea.KeyDown, tea.ModAlt}:  `\e[1;3B`,
	{tea.KeyRight, tea.ModAlt}: `\e[1;3C`,
	{tea.KeyLeft, tea.ModAlt}:  `\e[1;3D`,

	{tea.KeyUp, tea.ModShift}:    `\e[1;2A`,
	{tea.KeyDown, tea.ModShift}:  `\e[1;2B`,
	{tea.KeyRight, tea.ModShift}: `\e[1;2C`,
	{tea.KeyLeft, tea.ModShift}:  `\e[1;2D`,

	{tea.KeyUp, tea.ModCtrl}:    `\e[1;5A`,
	{tea.KeyDown, tea.ModCtrl}:  `\e[1;5B`,
	{tea.KeyRight, tea.ModCtrl}: `\e[1;5C`,
	{tea.KeyLeft, tea.ModCtrl}:  `\e[1;5D`,

	{tea.KeyUp, tea.ModCtrl | tea.ModAlt}:    `\e[1;7A`,
	{tea.KeyDown, tea.ModCtrl | tea.ModAlt}:  `\e[1;7B`,
	{tea.KeyRight, tea.ModCtrl | tea.ModAlt}: `\e[1;7C`,
	{tea.KeyLeft, tea.ModCtrl | tea.ModAlt}:  `\e[1;7D`,

	{tea.KeyUp, tea.ModCtrl | tea.ModShift}:    `\e[1;6A`,
	{tea.KeyDown, tea.ModCtrl | tea.ModShift}:  `\e[1;6B`,
	{tea.KeyRight, tea.ModCtrl | tea.ModShift}: `\e[1;6C`,
	{tea.KeyLeft, tea.ModCtrl | tea.ModShift}:  `\e[1;6D`,

	{tea.KeyUp, tea.ModCtrl | tea.ModShift | tea.ModAlt}:    `\e[1;8A`,
	{tea.KeyDown, tea.ModCtrl | tea.ModShift | tea.ModAlt}:  `\e[1;8B`,
	{tea.KeyRight, tea.ModCtrl | tea.ModShift | tea.ModAlt}: `\e[1;8C`,
	{tea.KeyLeft, tea.ModCtrl | tea.ModShift | tea.ModAlt}:  `\e[1;8D`,

	{tea.KeyUp, tea.ModShift | tea.ModAlt}:    `\e[1;4A`,
	{tea.KeyDown, tea.ModShift | tea.ModAlt}:  `\e[1;4B`,
	{tea.KeyRight, tea.ModShift | tea.ModAlt}: `\e[1;4C`,
	{tea.KeyLeft, tea.ModShift | tea.ModAlt}:  `\e[1;4D`,

	// ── Home / End ──────────────────────────────────────────────────────────
	{tea.KeyHome, 0}:          `\e[H`,
	{tea.KeyEnd, 0}:           `\e[F`,
	{tea.KeyHome, tea.ModAlt}: `\e[1;3H`,
	{tea.KeyEnd, tea.ModAlt}:  `\e[1;3F`,

	{tea.KeyHome, tea.ModCtrl}:              `\e[1;5H`,
	{tea.KeyEnd, tea.ModCtrl}:               `\e[1;5F`,
	{tea.KeyHome, tea.ModCtrl | tea.ModAlt}: `\e[1;7H`,
	{tea.KeyEnd, tea.ModCtrl | tea.ModAlt}:  `\e[1;7F`,

	{tea.KeyHome, tea.ModShift}:              `\e[1;2H`,
	{tea.KeyEnd, tea.ModShift}:               `\e[1;2F`,
	{tea.KeyHome, tea.ModShift | tea.ModAlt}: `\e[1;4H`,
	{tea.KeyEnd, tea.ModShift | tea.ModAlt}:  `\e[1;4F`,

	{tea.KeyHome, tea.ModCtrl | tea.ModShift}:              `\e[1;6H`,
	{tea.KeyEnd, tea.ModCtrl | tea.ModShift}:               `\e[1;6F`,
	{tea.KeyHome, tea.ModCtrl | tea.ModShift | tea.ModAlt}: `\e[1;8H`,
	{tea.KeyEnd, tea.ModCtrl | tea.ModShift | tea.ModAlt}:  `\e[1;8F`,

	// ── Page Up / Down ──────────────────────────────────────────────────────
	{tea.KeyPgUp, 0}:            `\e[5~`,
	{tea.KeyPgDown, 0}:          `\e[6~`,
	{tea.KeyPgUp, tea.ModAlt}:   `\e[5;3~`,
	{tea.KeyPgDown, tea.ModAlt}: `\e[6;3~`,

	{tea.KeyPgUp, tea.ModCtrl}:                `\e[5;5~`,
	{tea.KeyPgDown, tea.ModCtrl}:              `\e[6;5~`,
	{tea.KeyPgUp, tea.ModCtrl | tea.ModAlt}:   `\e[5;7~`,
	{tea.KeyPgDown, tea.ModCtrl | tea.ModAlt}: `\e[6;7~`,

	// ── Insert / Delete ─────────────────────────────────────────────────────
	{tea.KeyInsert, 0}:          `\e[2~`,
	{tea.KeyDelete, 0}:          `\e[3~`,
	{tea.KeyInsert, tea.ModAlt}: `\e[2;3~`,
	{tea.KeyDelete, tea.ModAlt}: `\e[3;3~`,

	// ── Shift+Tab ───────────────────────────────────────────────────────────
	{tea.KeyTab, tea.ModShift}: `\e[Z`,

	// ── Function keys (xterm/vt100 canonical sequences) ─────────────────────
	{tea.KeyF1, 0}:  `\eOP`,
	{tea.KeyF2, 0}:  `\eOQ`,
	{tea.KeyF3, 0}:  `\eOR`,
	{tea.KeyF4, 0}:  `\eOS`,
	{tea.KeyF5, 0}:  `\e[15~`,
	{tea.KeyF6, 0}:  `\e[17~`,
	{tea.KeyF7, 0}:  `\e[18~`,
	{tea.KeyF8, 0}:  `\e[19~`,
	{tea.KeyF9, 0}:  `\e[20~`,
	{tea.KeyF10, 0}: `\e[21~`,
	{tea.KeyF11, 0}: `\e[23~`,
	{tea.KeyF12, 0}: `\e[24~`,
	{tea.KeyF13, 0}: `\e[1;2P`,
	{tea.KeyF14, 0}: `\e[1;2Q`,
	{tea.KeyF15, 0}: `\e[1;2R`,
	{tea.KeyF16, 0}: `\e[1;2S`,
	{tea.KeyF17, 0}: `\e[15;2~`,
	{tea.KeyF18, 0}: `\e[17;2~`,
	{tea.KeyF19, 0}: `\e[18;2~`,
	{tea.KeyF20, 0}: `\e[19;2~`,

	{tea.KeyF1, tea.ModAlt}:  `\e[1;3P`,
	{tea.KeyF2, tea.ModAlt}:  `\e[1;3Q`,
	{tea.KeyF3, tea.ModAlt}:  `\e[1;3R`,
	{tea.KeyF4, tea.ModAlt}:  `\e[1;3S`,
	{tea.KeyF5, tea.ModAlt}:  `\e[15;3~`,
	{tea.KeyF6, tea.ModAlt}:  `\e[17;3~`,
	{tea.KeyF7, tea.ModAlt}:  `\e[18;3~`,
	{tea.KeyF8, tea.ModAlt}:  `\e[19;3~`,
	{tea.KeyF9, tea.ModAlt}:  `\e[20;3~`,
	{tea.KeyF10, tea.ModAlt}: `\e[21;3~`,
	{tea.KeyF11, tea.ModAlt}: `\e[23;3~`,
	{tea.KeyF12, tea.ModAlt}: `\e[24;3~`,
	{tea.KeyF13, tea.ModAlt}: `\e[25;3~`,
	{tea.KeyF14, tea.ModAlt}: `\e[26;3~`,
	{tea.KeyF15, tea.ModAlt}: `\e[28;3~`,
	{tea.KeyF16, tea.ModAlt}: `\e[29;3~`,
}
