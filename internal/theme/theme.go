package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const DefaultName = "default"

// Palette defines the shared color roles used by the TUIs.
type Palette struct {
	Name    string
	Label   string
	Accent  lipgloss.AdaptiveColor
	Border  lipgloss.AdaptiveColor
	Muted   lipgloss.AdaptiveColor
	Success lipgloss.AdaptiveColor
	Error   lipgloss.AdaptiveColor
}

var palettes = []Palette{
	{
		Name:    DefaultName,
		Label:   "Default",
		Accent:  lipgloss.AdaptiveColor{Light: "#5B8DD9", Dark: "#7AA2F7"},
		Border:  lipgloss.AdaptiveColor{Light: "#888888", Dark: "#555555"},
		Muted:   lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#555555"},
		Success: lipgloss.AdaptiveColor{Light: "#27AE60", Dark: "#98C379"},
		Error:   lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#FF5F87"},
	},
	{
		Name:    "ocean",
		Label:   "Ocean",
		Accent:  lipgloss.AdaptiveColor{Light: "#0F6CBD", Dark: "#4DB6FF"},
		Border:  lipgloss.AdaptiveColor{Light: "#6F8FA8", Dark: "#47667F"},
		Muted:   lipgloss.AdaptiveColor{Light: "#7E97A8", Dark: "#5A7384"},
		Success: lipgloss.AdaptiveColor{Light: "#158F77", Dark: "#46D9B8"},
		Error:   lipgloss.AdaptiveColor{Light: "#D1495B", Dark: "#FF7A90"},
	},
	{
		Name:    "forest",
		Label:   "Forest",
		Accent:  lipgloss.AdaptiveColor{Light: "#2F7D4E", Dark: "#7FD18B"},
		Border:  lipgloss.AdaptiveColor{Light: "#7D907D", Dark: "#506450"},
		Muted:   lipgloss.AdaptiveColor{Light: "#819181", Dark: "#5C6B5C"},
		Success: lipgloss.AdaptiveColor{Light: "#2C9B5F", Dark: "#7EE0A0"},
		Error:   lipgloss.AdaptiveColor{Light: "#C85C3B", Dark: "#FF9B73"},
	},
	{
		Name:    "sunset",
		Label:   "Sunset",
		Accent:  lipgloss.AdaptiveColor{Light: "#C65D3A", Dark: "#FF9A62"},
		Border:  lipgloss.AdaptiveColor{Light: "#9E7B73", Dark: "#6B5752"},
		Muted:   lipgloss.AdaptiveColor{Light: "#A4877D", Dark: "#725E59"},
		Success: lipgloss.AdaptiveColor{Light: "#A56A14", Dark: "#FFC368"},
		Error:   lipgloss.AdaptiveColor{Light: "#C53A5D", Dark: "#FF6E91"},
	},
	{
		Name:    "mono",
		Label:   "Mono",
		Accent:  lipgloss.AdaptiveColor{Light: "#444444", Dark: "#E0E0E0"},
		Border:  lipgloss.AdaptiveColor{Light: "#808080", Dark: "#666666"},
		Muted:   lipgloss.AdaptiveColor{Light: "#9A9A9A", Dark: "#707070"},
		Success: lipgloss.AdaptiveColor{Light: "#5F5F5F", Dark: "#BDBDBD"},
		Error:   lipgloss.AdaptiveColor{Light: "#2E2E2E", Dark: "#F0F0F0"},
	},
}

func Normalize(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return DefaultName
	}
	return name
}

func IsValid(name string) bool {
	name = Normalize(name)
	for _, p := range palettes {
		if p.Name == name {
			return true
		}
	}
	return false
}

func Names() []string {
	names := make([]string, 0, len(palettes))
	for _, p := range palettes {
		names = append(names, p.Name)
	}
	return names
}

func Label(name string) string {
	return Get(name).Label
}

func Get(name string) Palette {
	name = Normalize(name)
	for _, p := range palettes {
		if p.Name == name {
			return p
		}
	}
	return palettes[0]
}

func All() []Palette {
	out := make([]Palette, len(palettes))
	copy(out, palettes)
	return out
}
