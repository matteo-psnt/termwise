package configui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestRenderModelListScrolls(t *testing.T) {
	list := make([]provider.Model, 30)
	for i := range list {
		list[i].ID = fmt.Sprintf("model-%02d", i)
	}
	styles := newStylesForTheme(true, "")

	// height 20 leaves room for 6 rows.
	out := renderModelList(styles, list, 29, 20, "model-27")
	if !strings.Contains(out, "↑ 24 more") || strings.Contains(out, "↓") {
		t.Errorf("want only an up marker for 24 hidden rows:\n%s", out)
	}
	if strings.Contains(out, "model-23") || !strings.Contains(out, "model-24") {
		t.Errorf("window should start at model-24:\n%s", out)
	}
	if !strings.Contains(out, "(recommended)") {
		t.Errorf("recommended model not badged:\n%s", out)
	}
}
