package cliname

import "testing"

func TestName(t *testing.T) {
	tests := map[string]string{
		"":                  Default,
		"tw":                "tw",
		"my-tw":             "my-tw",
		"tw config; rm -rf": Default, // not a plain command word
		"tw\nrm":            Default,
		"$(whoami)":         Default,
	}
	for env, want := range tests {
		t.Setenv(EnvVar, env)
		if got := Name(); got != want {
			t.Errorf("Name() with %s=%q = %q, want %q", EnvVar, env, got, want)
		}
	}
}

// The shell templates interpolate EnvVar, so they follow a rename, but shells
// that are already open keep exporting the old name until the user starts a
// new one. Until then every message quietly reverts to "termwise" for users
// who do have the integration, so a rename should be deliberate.
func TestShellTemplatesSetTheEnvVar(t *testing.T) {
	// Kept as a literal so the test fails on a rename rather than following it.
	const literal = "TERMWISE_INVOKED_AS"
	if EnvVar != literal {
		t.Fatalf("EnvVar = %q; shells already running the integration set %q", EnvVar, literal)
	}
}
