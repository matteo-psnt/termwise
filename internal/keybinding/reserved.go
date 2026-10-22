package keybinding

// reservedKeys are the bindings termwise refuses to take over, with why. Each
// is a key the terminal or line editor needs for basic shell use, so binding
// it would break typing commands rather than add a shortcut. The list is kept
// to those: plenty of other defaults (^A, ^E, ^R…) are rebindable at the
// user's own choice.
var reservedKeys = map[string]string{
	"^M": "it is Enter",
	"^J": "it is Enter (newline)",
	"^I": "it is Tab, used for completion",
	"^[": "it is Esc, which starts every Alt and arrow key",
	"^H": "it is Backspace",
	"^?": "it is Backspace",
	"^C": "it interrupts the running command",
	"^D": "it ends input and closes the shell",
	"^Z": "it suspends the running command",
	"^S": "it is flow control (stop), which most terminals swallow",
	"^Q": "it is flow control (resume), which most terminals swallow",
	// Alt+[ and Alt+O send the prefixes of the arrow, Home/End and function
	// key sequences; binding them as keys of their own breaks those keys in
	// readline.
	`\e[`: "it starts the sequences arrow and function keys send",
	`\eO`: "it starts the sequences arrow and function keys send",
}

// Reserved reports whether binding (a stored zsh bindkey string) is a key the
// shell needs for basic editing, and if so a short reason to show the user.
func Reserved(binding string) (reason string, reserved bool) {
	reason, reserved = reservedKeys[binding]
	return reason, reserved
}
