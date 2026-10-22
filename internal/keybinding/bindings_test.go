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

func TestToBash(t *testing.T) {
	cases := map[string]string{
		"^T":     `\C-t`,
		"^@":     `\C-@`,
		"^?":     `\C-?`,
		`^\`:     `\034`,
		"^[":     `\e`,
		"^]":     `\C-]`,
		"^^":     `\C-^`,
		"^_":     `\C-_`,
		`\e^X`:   `\e\C-x`,
		`\e^_`:   `\e\C-_`,
		`\ef`:    `\ef`,
		`\e[15~`: `\e[15~`,
	}
	for in, want := range cases {
		if got := ToBash(in); got != want {
			t.Errorf("ToBash(%q) = %q; want %q", in, got, want)
		}
	}
}
