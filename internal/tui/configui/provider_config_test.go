package configui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
	_ "github.com/matteo-psnt/termwise/internal/provider/builtin"
)

func TestBuildProviderConfigFromAuthInput(t *testing.T) {
	tests := []struct {
		name             string
		provider, method string
		input, fallback  string
		want             config.ProviderConfig
	}{
		{"env typed", "openai", "env", "MY_KEY", "OPENAI_API_KEY",
			config.ProviderConfig{Auth: "env", EnvVar: "MY_KEY"}},
		{"env falls back", "openai", "env", "", "OPENAI_API_KEY",
			config.ProviderConfig{Auth: "env", EnvVar: "OPENAI_API_KEY"}},
		{"cmd", "openai", "cmd", "pass show k", "",
			config.ProviderConfig{Auth: "cmd", APIKeyCmd: "pass show k"}},
		{"keychain never stores the raw key", "openai", "keychain", "sk-secret", "",
			config.ProviderConfig{Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai")}},
		{"no-auth default URL omitted", "ollama", "env", "", "http://localhost:11434",
			config.ProviderConfig{Auth: "env"}},
		{"no-auth custom URL kept", "ollama", "env", "http://box:11434", "http://localhost:11434",
			config.ProviderConfig{Auth: "env", BaseURL: "http://box:11434"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProviderConfigFromAuthInput(tt.provider, tt.method, tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
