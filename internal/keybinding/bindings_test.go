package keybinding

import "testing"

func TestToFish(t *testing.T) {
	ok := map[string]string{
		"^T":     "ctrl-t",
		"^K":     "ctrl-k",
		`\ef`:    "alt-f",
		`\e[15~`: "f5",
		`\e^X`:   "alt-ctrl-x",
	}
	for in, want := range ok {
		got, valid := ToFish(in)
		if !valid || got != want {
			t.Errorf("ToFish(%q) = %q,%v; want %q,true", in, got, valid, want)
		}
	}
	// Sequences with no fish 4 name must be rejected rather than echoed.
	for _, in := range []string{"^@", `\e[99~`, "garbage"} {
		if got, valid := ToFish(in); valid {
			t.Errorf("ToFish(%q) = %q,true; want ok=false", in, got)
		}
	}
}
