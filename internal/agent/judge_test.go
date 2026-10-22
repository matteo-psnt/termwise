package agent

import "testing"

func TestIsSafeVerdict(t *testing.T) {
	for reply, want := range map[string]bool{
		"safe":            true,
		"Safe.":           true,
		" **SAFE** \n":    true,
		"`safe`":          true,
		"unsafe":          false,
		"Unsafe.":         false,
		"safe, but risky": false,
		"":                false,
	} {
		if got := isSafeVerdict(reply); got != want {
			t.Errorf("isSafeVerdict(%q) = %v, want %v", reply, got, want)
		}
	}
}
