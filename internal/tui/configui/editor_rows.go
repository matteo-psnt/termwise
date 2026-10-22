package configui

import (
	"sort"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
)

// ---------------------------------------------------------------------------
// Row types
// ---------------------------------------------------------------------------

type editorRowKind int

const (
	rowSectionHeader editorRowKind = iota
	rowProvHeader
	rowProvModel
	rowProvEffort
	rowProvAuth
	rowAddProvider
	rowSetting // driven by editorSettings table; settingIdx identifies which one
)

type editorRow struct {
	kind       editorRowKind
	provider   string
	label      string
	settingIdx int    // only meaningful when kind == rowSetting
	desc       string // help text shown above hint when focused (may be empty)
	hint       string // keyboard hint shown when focused
}

// ---------------------------------------------------------------------------
// Row list management
// ---------------------------------------------------------------------------

func (m *editorModel) buildRows() {
	m.rows = []editorRow{{kind: rowSectionHeader, label: "Providers"}}
	for _, name := range m.sortedProviders() {
		m.rows = append(m.rows,
			editorRow{kind: rowProvHeader, provider: name, hint: "enter set active   " + navTail},
			editorRow{kind: rowProvModel, provider: name, hint: "enter pick model   " + navTail},
		)
		if models.SupportsThinking(name, m.cfg.Providers[name].Model) {
			m.rows = append(m.rows, editorRow{
				kind:     rowProvEffort,
				provider: name,
				desc:     "Reasoning effort level used for this model.",
				hint:     "enter pick   " + navTail,
			})
		}
		m.rows = append(m.rows,
			editorRow{kind: rowProvAuth, provider: name, hint: "enter edit auth   " + navTail},
		)
	}
	if len(m.cfg.Providers) < len(config.ProviderInfos()) {
		m.rows = append(m.rows, editorRow{kind: rowAddProvider, hint: "enter add provider   " + navTail})
	}
	m.rows = append(m.rows, editorRow{kind: rowSectionHeader, label: "Settings"})
	for i, s := range config.Settings {
		m.rows = append(m.rows, editorRow{
			kind:       rowSetting,
			settingIdx: i,
			desc:       s.Description,
			hint:       hintForKind(s.Kind),
		})
	}
	m.snapCursor()
}

func (m *editorModel) snapCursor() {
	for m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowSectionHeader {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m editorModel) moveCursor(delta int) editorModel {
	next := m.cursor + delta
	for next >= 0 && next < len(m.rows) {
		if m.rows[next].kind != rowSectionHeader {
			m.cursor = next
			return m
		}
		next += delta
	}
	return m
}

func (m editorModel) sortedProviders() []string {
	names := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
