package config

// FileConfig is the in-memory representation of config.toml.
// It maps 1:1 with what is persisted to disk — no runtime state, derived
// values, or session data belongs here.
type FileConfig struct {
	SelectedProvider string                    `toml:"selected_provider,omitempty"`
	Providers        map[string]ProviderConfig `toml:"providers,omitempty"`
	UI               UIConfig                  `toml:"ui,omitempty"`
	Shell            ShellConfig               `toml:"shell,omitempty"`
	Policies         PoliciesConfig            `toml:"policies,omitempty"`
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

// UIConfig holds display preferences.
type UIConfig struct {
	Theme      string `toml:"theme,omitempty"`
	ShowFooter *bool  `toml:"show_footer,omitempty"`
}

// ShellConfig holds shell integration preferences.
type ShellConfig struct {
	Keybinding string `toml:"keybinding,omitempty"`
}

// PoliciesConfig holds tool-behaviour policies.
type PoliciesConfig struct {
	Bash BashPolicyConfig `toml:"bash,omitempty"`
}

// BashPolicyConfig holds policies for the bash tool.
type BashPolicyConfig struct {
	LLMJudge bool `toml:"llm_judge,omitempty"`
}
