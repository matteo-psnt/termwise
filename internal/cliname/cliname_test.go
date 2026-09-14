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

// The shell templates set the variable this package reads. If either side is
// renamed without the other, every message quietly reverts to "termwise" for
// users who do have the integration — which nothing else would catch.
func TestShellTemplatesSetTheEnvVar(t *testing.T) {
	// Kept as a literal so the test fails on a rename rather than following it.
	const literal = "TERMWISE_INVOKED_AS"
	if EnvVar != literal {
		t.Fatalf("EnvVar = %q; the shell templates in cmd/termwise/initcmd.go set %q", EnvVar, literal)
	}
}
