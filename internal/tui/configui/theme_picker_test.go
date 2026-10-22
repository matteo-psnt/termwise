package configui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/theme"
)

func TestThemePickerCursorOutOfRange(t *testing.T) {
	m := themePickerModel{cursor: len(theme.All()), prev: theme.DefaultName}
	_ = m.View(newStylesForTheme(true, ""), true) // must not panic

	m, _ = m.Update(keyEnter)
	if !m.done || m.result != theme.DefaultName {
		t.Errorf("done = %v, result = %q; want the original theme", m.done, m.result)
	}
}
