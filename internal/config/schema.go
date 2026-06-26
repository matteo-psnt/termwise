package config

// FileConfig is the in-memory representation of config.toml.
// It maps 1:1 with what is persisted to disk — no runtime state, derived
// values, or session data belongs here.
type FileConfig struct {
	SelectedProvider string                    `toml:"selected_provider,omitempty"`
	Providers        map[string]ProviderConfig `toml:"providers,omitempty"`
	Settings         SettingsConfig            `toml:"settings,omitempty"`
}

// ProviderConfig holds per-provider connection, auth, and model settings.
type ProviderConfig struct {
	// Auth selects how the API key is resolved: "env", "cmd", or "keychain".
	// Defaults to "env" when empty.
	Auth          string `toml:"auth,omitempty"`
	EnvVar        string `toml:"env_var,omitempty"`
	APIKeyCmd     string `toml:"api_key_cmd,omitempty"`
	KeychainEntry string `toml:"keychain_entry,omitempty"`

	Model   string `toml:"model,omitempty"`
	BaseURL string `toml:"base_url,omitempty"`
}

// SettingsConfig holds all user-facing preferences in a single flat block.
// Boolean settings use *bool so nil (never written) is distinguishable from
// an explicit false, allowing each setting to declare its own default.
type SettingsConfig struct {
	Theme       string `toml:"theme,omitempty"`
	Keybinding  string `toml:"keybinding,omitempty"`
	LLMJudge    *bool  `toml:"llm_judge,omitempty"`
	AutoResume  *bool  `toml:"auto_resume,omitempty"`
	Suggestions *bool  `toml:"suggestions,omitempty"`
}
