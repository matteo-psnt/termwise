package tools

import (
	"strings"
	"testing"
)

// The command tool is told to fill in real paths from the environment block,
// which invited it to prefix commands with a cd into the working directory it
// was already in. Observed in a real turn: "cd /long/abs/path && git reset …".
func TestCommandDefForbidsRedundantCd(t *testing.T) {
	d := CommandDef.Description
	for _, want := range []string{
		"current working directory",
		"Never prefix it with a cd",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("command tool description missing %q", want)
		}
	}
}

func TestCommandDefStillForbidsPlaceholders(t *testing.T) {
	if !strings.Contains(CommandDef.Description, "No placeholders") {
		t.Error("placeholder rule was lost")
	}
}
