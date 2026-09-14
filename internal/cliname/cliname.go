// Package cliname reports the command name to use when a message tells the
// user to go and run something else.
//
// It exists because there are two of them. The binary is `termwise`; `tw` is a
// shell function the integration defines, and it does not exist until the user
// has set that up — which is exactly the state they are in when they first hit
// "no configuration found". Messages that hardcoded `tw` sent those users to a
// command their shell had never heard of.
//
// The binary cannot work this out for itself: the wrapper calls
// `command termwise`, so os.Args[0] is the same either way. The integration
// announces itself with an environment variable instead.
package cliname

import "os"

// EnvVar is exported so the shell templates that set it and the code that
// reads it cannot drift apart.
const EnvVar = "TERMWISE_INVOKED_AS"

// Default is the binary's own name, which is always a valid thing to type.
const Default = "termwise"

// Name returns the short name when the shell integration is loaded, and the
// binary name otherwise.
//
// The value is pasted into user-facing text, so anything that is not a plain
// command word is ignored rather than echoed back.
func Name() string {
	n := os.Getenv(EnvVar)
	if n == "" || len(n) > 32 {
		return Default
	}
	for _, r := range n {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return Default
		}
	}
	return n
}
