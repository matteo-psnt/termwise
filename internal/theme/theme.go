// Package theme defines the named color palettes shared by the TUIs. It holds
// only hex strings, never lipgloss styles, so config and other non-UI code can
// validate and list themes without pulling in a rendering dependency.
package theme

import "strings"

// DefaultName is the palette used when no theme is configured or the
// configured one is unknown.
const DefaultName = "default"

// Color is a light/dark hex pair. The concrete color is chosen at render time
// via lipgloss.LightDark once the terminal background is known, so this package
// stays free of any lipgloss dependency.
type Color struct {
	Light string
	Dark  string
}

// Pick returns the Dark value on a dark background and the Light value
// otherwise.
func (c Color) Pick(dark bool) string {
	if dark {
		return c.Dark
	}
	return c.Light
}

// Palette defines the shared color roles used by the TUIs.
type Palette struct {
	Name    string
	Label   string
	Accent  Color
	Border  Color
	Muted   Color
	Success Color
	Error   Color
}

var palettes = []Palette{
	{
		Name:    DefaultName,
		Label:   "Default",
		Accent:  Color{Light: "#5B8DD9", Dark: "#7AA2F7"},
		Border:  Color{Light: "#888888", Dark: "#555555"},
		Muted:   Color{Light: "#AAAAAA", Dark: "#555555"},
		Success: Color{Light: "#27AE60", Dark: "#98C379"},
		Error:   Color{Light: "#E06C75", Dark: "#FF5F87"},
	},
	{
		Name:    "ocean",
		Label:   "Ocean",
		Accent:  Color{Light: "#0F6CBD", Dark: "#4DB6FF"},
		Border:  Color{Light: "#6F8FA8", Dark: "#47667F"},
		Muted:   Color{Light: "#7E97A8", Dark: "#5A7384"},
		Success: Color{Light: "#158F77", Dark: "#46D9B8"},
		Error:   Color{Light: "#D1495B", Dark: "#FF7A90"},
	},
	{
		Name:    "forest",
		Label:   "Forest",
		Accent:  Color{Light: "#2F7D4E", Dark: "#7FD18B"},
		Border:  Color{Light: "#7D907D", Dark: "#506450"},
		Muted:   Color{Light: "#819181", Dark: "#5C6B5C"},
		Success: Color{Light: "#2C9B5F", Dark: "#7EE0A0"},
		Error:   Color{Light: "#C85C3B", Dark: "#FF9B73"},
	},
	{
		Name:    "sunset",
		Label:   "Sunset",
		Accent:  Color{Light: "#C65D3A", Dark: "#FF9A62"},
		Border:  Color{Light: "#9E7B73", Dark: "#6B5752"},
		Muted:   Color{Light: "#A4877D", Dark: "#725E59"},
		Success: Color{Light: "#A56A14", Dark: "#FFC368"},
		Error:   Color{Light: "#C53A5D", Dark: "#FF6E91"},
	},
	{
		Name:    "mono",
		Label:   "Mono",
		Accent:  Color{Light: "#444444", Dark: "#E0E0E0"},
		Border:  Color{Light: "#808080", Dark: "#666666"},
		Muted:   Color{Light: "#9A9A9A", Dark: "#707070"},
		Success: Color{Light: "#5F5F5F", Dark: "#BDBDBD"},
		Error:   Color{Light: "#2E2E2E", Dark: "#F0F0F0"},
	},
}

// Normalize lowercases and trims a theme name, so config values match however
// they were typed. An empty name means DefaultName.
func Normalize(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return DefaultName
	}
	return name
}

// IsValid reports whether name, once normalized, names a known palette. Get
// never fails, so callers that must reject a bad name (config validation)
// check here first.
func IsValid(name string) bool {
	name = Normalize(name)
	for _, p := range palettes {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Names returns the palette names in display order.
func Names() []string {
	names := make([]string, 0, len(palettes))
	for _, p := range palettes {
		names = append(names, p.Name)
	}
	return names
}

// Label returns the display label for name, or the default palette's label if
// name is unknown.
func Label(name string) string {
	return Get(name).Label
}

// Get returns the palette for name. An unknown name falls back to the default
// palette rather than failing: a stale or mistyped theme in the config file
// should not stop the UI from starting.
func Get(name string) Palette {
	name = Normalize(name)
	for _, p := range palettes {
		if p.Name == name {
			return p
		}
	}
	return palettes[0]
}

// All returns every palette in display order. The slice is a copy, so callers
// cannot modify the built-in palettes.
func All() []Palette {
	out := make([]Palette, len(palettes))
	copy(out, palettes)
	return out
}
