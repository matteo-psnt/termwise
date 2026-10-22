package agentui

import (
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// applyModel swaps the active provider client to the given provider+model
// in memory only. Existing conversation messages are preserved. Use this for
// live preview while a picker is open; pair with persistActiveModel to write
// the selection to disk on commit.
func (m *Model) applyModel(providerName, modelID string) error {
	if m.cfgPath == "" {
		return fmt.Errorf("no config path available to resolve auth for %q", providerName)
	}
	cfg, exists, err := config.LoadConfig(m.cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !exists {
		return fmt.Errorf("config file not found")
	}
	pc, ok := cfg.Providers[providerName]
	if !ok {
		return fmt.Errorf("provider %q has no config block — run `%s config` to add it", providerName, cliname.Name())
	}
	auth, err := config.ResolveAuth(providerName, pc)
	if err != nil {
		return fmt.Errorf("resolving auth for %q: %w", providerName, err)
	}
	client, err := config.NewClientFromResolved(config.ResolvedConfig{
		ProviderName: providerName,
		Model:        modelID,
		APIKey:       auth.APIKey,
		BaseURL:      auth.BaseURL,
	})
	if err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	m.provider = client
	m.providerName = providerName
	m.modelID = modelID
	m.effort = pc.Effort
	m.contextWindow = contextWindowFor(providerName, modelID)
	return nil
}

// defaultContextWindow is assumed for models the catalog does not know, or
// lists without a context size. Erring small is the safe direction: it only
// makes trimContext drop old tool results sooner.
const defaultContextWindow = 32_000

// contextWindowFor returns the context window, in tokens, of a provider's model.
func contextWindowFor(providerName, modelID string) int {
	if md := models.Find(providerName, modelID); md != nil && md.Context > 0 {
		return md.Context
	}
	return defaultContextWindow
}

// persistActiveModel writes the currently active provider+model to the config
// file as the selected provider and that provider's default model.
func (m *Model) persistActiveModel() {
	m.updateConfigFile(func(cfg *config.FileConfig) {
		pc := cfg.Providers[m.providerName]
		pc.Model = m.modelID
		cfg.Providers[m.providerName] = pc
		cfg.SelectedProvider = m.providerName
	})
}

// applyTheme rebuilds the renderer with the given theme name.
func (m *Model) applyTheme(name string) {
	m.themeName = name
	m.renderer = newRenderer(m.hasDarkBg, theme.Get(name), m.renderWidth())
	m.input.SetStyles(inputStyles(m.hasDarkBg, m.renderer.styles.Suggestion))
}

func (m Model) modelShortName() string {
	md := models.Find(m.providerName, m.modelID)
	if md == nil {
		return m.modelID
	}
	return strings.TrimPrefix(md.Name, "Claude ")
}
