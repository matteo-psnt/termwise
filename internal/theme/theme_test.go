package theme

import "testing"

func TestColorPick(t *testing.T) {
	c := Color{Light: "#ffffff", Dark: "#000000"}
	if got := c.Pick(true); got != c.Dark {
		t.Fatalf("Pick(true) = %q, want %q", got, c.Dark)
	}
	if got := c.Pick(false); got != c.Light {
		t.Fatalf("Pick(false) = %q, want %q", got, c.Light)
	}
}
