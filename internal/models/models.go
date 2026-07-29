package models

import (
	_ "embed"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// ModelData holds metadata for a single model.
type ModelData struct {
	ID               string  `yaml:"id"`
	Name             string  `yaml:"name"`
	Context          int     `yaml:"context"`      // tokens
	InputPrice       float64 `yaml:"input_price"`  // per 1M tokens
	OutputPrice      float64 `yaml:"output_price"` // per 1M tokens
	SupportsTools    bool    `yaml:"supports_tools"`
	SupportsThinking bool    `yaml:"supports_thinking"` // adaptive thinking / reasoning effort
}

// ProviderData holds all model metadata for one provider.
type ProviderData struct {
	DefaultModel string      `yaml:"default_model"`
	Models       []ModelData `yaml:"models"`
}

// modelsFile is the top-level structure of models.yaml.
type modelsFile struct {
	Providers map[string]ProviderData `yaml:"providers"`
}

// ----------------------------------------------------------------------------
// Package-level loaded data
// ----------------------------------------------------------------------------

var (
	once    sync.Once
	loaded  modelsFile
	loadErr error
)

//go:embed models.yaml
var builtinModelsYAML []byte

func load() {
	once.Do(func() {
		if err := yaml.Unmarshal(builtinModelsYAML, &loaded); err != nil {
			loadErr = fmt.Errorf("failed to parse embedded models.yaml: %w", err)
		}
	})
}

// DefaultModel returns the default model ID for a provider.
// Returns "" if the provider is unknown or has no default set.
func DefaultModel(provider string) string {
	load()
	pd, ok := loaded.Providers[provider]
	if !ok {
		return ""
	}
	return pd.DefaultModel
}

// Provider returns all data for a provider. ok is false if not found.
func Provider(name string) (ProviderData, bool) {
	load()
	pd, ok := loaded.Providers[name]
	return pd, ok
}

// SupportsThinking reports whether the given model supports adaptive thinking /
// reasoning effort. Unknown models return false.
func SupportsThinking(provider, modelID string) bool {
	md := Find(provider, modelID)
	return md != nil && md.SupportsThinking
}

// Find returns metadata for a specific model within a provider.
// Returns nil if the provider or model is not found.
func Find(provider, modelID string) *ModelData {
	load()
	pd, ok := loaded.Providers[provider]
	if !ok {
		return nil
	}
	for i := range pd.Models {
		if pd.Models[i].ID == modelID {
			return &pd.Models[i]
		}
	}
	return nil
}

// KnownProviders returns the list of provider names in models.yaml.
func KnownProviders() []string {
	load()
	names := make([]string, 0, len(loaded.Providers))
	for k := range loaded.Providers {
		names = append(names, k)
	}
	return names
}

// LoadErr returns any error encountered parsing the embedded models.yaml.
// This should only ever be non-nil if the embedded file is malformed (a bug).
func LoadErr() error {
	load()
	return loadErr
}
