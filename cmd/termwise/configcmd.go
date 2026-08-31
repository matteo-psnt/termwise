package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/tui/configui"
)

var configCmd = &cobra.Command{
	Use:           "config",
	Short:         "Manage termwise configuration",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		if cfgPath, err := config.DefaultConfigPath(); err == nil {
			emitConfigWarnings(cfgPath)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Any positional args mean cobra didn't match a subcommand — show help
		// rather than trying to open the TUI with garbage input.
		if len(args) > 0 {
			return cmd.Help()
		}
		if edit, _ := cmd.Flags().GetBool("edit"); edit {
			return editConfig()
		}
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		cfg, exists, err := config.LoadConfig(cfgPath)
		if err != nil {
			// Config exists but failed to parse — open it in $EDITOR so the
			// user can repair it directly. The TUI is recovery, not a dead end.
			fmt.Fprintf(os.Stderr, "Warning: %s\n\nOpening file for repair — fix it and save, or delete it to start fresh.\n\n", err)
			return editConfig()
		}
		return configui.Open(cfgPath, cfg, exists)
	},
}

// ---------------------------------------------------------------------------
// tw config show
// ---------------------------------------------------------------------------

var configShowCmd = &cobra.Command{
	Use:          "show",
	Short:        "Print the effective configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		cfg, exists, err := config.LoadConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s\n  Run `tw config --edit` to repair the file.\n", err)
			return nil
		}
		if !exists {
			fmt.Fprintln(os.Stderr, "No configuration file — run `tw config` to set up.")
			return nil
		}
		if asJSON {
			return printShowJSON(cfgPath, cfg)
		}
		return printShow(cfgPath, cfg)
	},
}

// ---------------------------------------------------------------------------
// tw config use <provider>
// ---------------------------------------------------------------------------

var configUseCmd = &cobra.Command{
	Use:          "use <provider>",
	Short:        "Set the active provider",
	Example:      "  tw config use anthropic\n  tw config use openai",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateConfig(func(cfg *config.FileConfig) error {
			return config.UseProvider(cfg, args[0])
		})
	},
}

// ---------------------------------------------------------------------------
// tw config model
// ---------------------------------------------------------------------------

var configModelCmd = &cobra.Command{
	Use:          "model",
	Short:        "Manage models",
	SilenceUsage: true,
}

var configModelListCmd = &cobra.Command{
	Use:          "list [provider]",
	Short:        "List available models for the selected (or given) provider",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		cfg, _, err := config.LoadConfig(cfgPath)
		if err != nil {
			return err
		}

		name := cfg.SelectedProvider
		if len(args) == 1 {
			name = args[0]
		}
		if name == "" {
			return fmt.Errorf("no provider selected — pass a provider name or run `tw config use <provider>`")
		}

		pc, ok := cfg.Providers[name]
		if !ok {
			return fmt.Errorf("provider %q is not configured — run `tw config` to set it up", name)
		}

		fmt.Fprintf(os.Stderr, "Fetching models for %s...", name)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		models, err := config.ListProviderModels(ctx, name, pc)
		fmt.Fprintf(os.Stderr, "\r\033[K")
		if err != nil {
			return fmt.Errorf("listing models: %w", err)
		}
		for _, m := range models {
			fmt.Println(m.ID)
		}
		return nil
	},
}

var configModelSetCmd = &cobra.Command{
	Use:          "set <model>",
	Short:        "Set the model for the selected provider",
	Example:      "  tw config model set claude-sonnet-4-6\n  tw config model set gpt-4o",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		modelID := args[0]
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			if err := config.SetModel(cfg, name, modelID); err != nil {
				return err
			}
			// Validate the model against the API, but don't fail the save on error.
			pc := cfg.Providers[name]
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			fmt.Fprintf(os.Stderr, "Validating model...")
			models, err := config.ListProviderModels(ctx, name, pc)
			fmt.Fprintf(os.Stderr, "\r\033[K")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not validate model for %s — saved anyway\n", name)
				return nil
			}
			for _, m := range models {
				if m.ID == modelID {
					return nil
				}
			}
			return fmt.Errorf("model %q not found for provider %q", modelID, name)
		})
	},
}

// ---------------------------------------------------------------------------
// tw config auth
// ---------------------------------------------------------------------------

var configAuthCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Configure authentication for the selected provider",
	SilenceUsage: true,
}

var configAuthEnvCmd = &cobra.Command{
	Use:          "env <ENV_VAR>",
	Short:        "Use an environment variable for auth",
	Example:      "  tw config auth env ANTHROPIC_API_KEY",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthEnv(cfg, name, args[0])
		})
	},
}

var configAuthShellCmd = &cobra.Command{
	Use:          "cmd <COMMAND>",
	Short:        "Use a shell command to retrieve the API key",
	Example:      "  tw config auth cmd \"op read 'op://personal/openai/api-key'\"",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthCmd(cfg, name, args[0])
		})
	},
}

var configAuthKeychainCmd = &cobra.Command{
	Use:          "keychain <ENTRY>",
	Short:        "Use a macOS Keychain entry for auth",
	Example:      "  tw config auth keychain termwise-anthropic",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthKeychain(cfg, name, args[0])
		})
	},
}

// ---------------------------------------------------------------------------
// tw config test
// ---------------------------------------------------------------------------

var configTestCmd = &cobra.Command{
	Use:          "test",
	Short:        "Test connectivity for the selected provider",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		cfg, _, err := config.LoadConfig(cfgPath)
		if err != nil {
			return err
		}
		name := cfg.SelectedProvider
		if name == "" {
			return fmt.Errorf("no provider selected — run `tw config use <provider>`")
		}
		pc, ok := cfg.Providers[name]
		if !ok {
			return fmt.Errorf("provider %q has no config block — run `tw config` to set it up", name)
		}

		fmt.Fprintf(os.Stderr, "Testing %s...", name)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := config.CheckProviderConnectivity(ctx, name, pc); err != nil {
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return fmt.Errorf("connection failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\r\033[K")
		fmt.Printf("✓ %s is reachable\n", name)
		return nil
	},
}

// ---------------------------------------------------------------------------
// tw config doctor
// ---------------------------------------------------------------------------

var configDoctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "Report configuration issues",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		if !runDoctor(cfgPath) {
			os.Exit(1)
		}
		return nil
	},
}

// ---------------------------------------------------------------------------
// tw config remove <provider>
// ---------------------------------------------------------------------------

var configRemoveCmd = &cobra.Command{
	Use:          "remove <provider>",
	Short:        "Remove a provider config block",
	Example:      "  tw config remove openai",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateConfig(func(cfg *config.FileConfig) error {
			return config.RemoveProvider(cfg, args[0])
		})
	},
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mutateConfig loads the config, applies fn, and saves atomically.
// A parse error is wrapped with a recovery hint so the user knows how to fix
// the file before retrying.
func mutateConfig(fn func(*config.FileConfig) error) error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("%w\n  Config file is invalid — run `tw config --edit` to repair it", err)
	}
	if err := fn(&cfg); err != nil {
		return err
	}
	return config.SaveConfig(cfgPath, cfg)
}

// mutateActiveProvider is mutateConfig scoped to the currently selected provider.
func mutateActiveProvider(fn func(*config.FileConfig, string) error) error {
	return mutateConfig(func(cfg *config.FileConfig) error {
		name := cfg.SelectedProvider
		if name == "" {
			return fmt.Errorf("no provider selected — run `tw config use <provider>` first")
		}
		return fn(cfg, name)
	})
}

// printShow writes a human-readable view of the full configuration to stdout.
func printShow(cfgPath string, cfg config.FileConfig) error {
	fmt.Printf("Config: %s\n", cfgPath)

	// Providers section.
	fmt.Println()
	fmt.Println("Providers:")
	if len(cfg.Providers) == 0 {
		fmt.Println("  (none configured)")
	} else {
		for _, name := range sortedProviderNames(cfg) {
			pc := cfg.Providers[name]
			active := name == cfg.SelectedProvider
			label := name
			if active {
				label += " (active)"
			}
			if active {
				fmt.Printf("* %s\n", label)
			} else {
				fmt.Printf("  %s\n", label)
			}

			model := pc.Model
			if model == "" {
				model = "(not set)"
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintf(w, "    Model:\t%s\n", model); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "    Auth:\t%s\n", authSummary(name, pc)); err != nil {
				return err
			}
			if pc.BaseURL != "" {
				if _, err := fmt.Fprintf(w, "    URL:\t%s\n", pc.BaseURL); err != nil {
					return err
				}
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Println()
		}
	}

	// Settings section — driven by the central registry.
	fmt.Println("Settings:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, def := range config.Settings {
		val := def.Get(cfg)
		if val == "" {
			resolved := def.Resolve(cfg)
			if resolved != "" {
				val = resolved + " (default)"
			}
		}
		if _, err := fmt.Fprintf(w, "  %s:\t%s\n", def.Label, val); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

// sortedProviderNames returns provider names with the active one first,
// then the rest in alphabetical order.
func sortedProviderNames(cfg config.FileConfig) []string {
	var names []string
	if _, ok := cfg.Providers[cfg.SelectedProvider]; ok {
		names = append(names, cfg.SelectedProvider)
	}
	rest := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		if name != cfg.SelectedProvider {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(names, rest...)
}

// runDoctor prints a per-check diagnostic report and returns true if all pass.
func runDoctor(cfgPath string) bool {
	allOk := true
	pass := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	fail := func(msg string) { fmt.Printf("  ✗ %s\n", msg); allOk = false }

	fmt.Printf("Config: %s\n\n", cfgPath)

	cfg, exists, parseErr := config.LoadConfig(cfgPath)
	if parseErr != nil {
		fail("File: " + parseErr.Error())
		fmt.Println("    → Run `tw config --edit` to repair")
		return false
	}
	if !exists {
		fail("File: not found")
		fmt.Println("    → Run `tw config` to create it")
		return false
	}
	pass("File: exists and valid TOML")

	// selected_provider set and known.
	if cfg.SelectedProvider == "" {
		fail("selected_provider: not set")
		fmt.Println("    → Run `tw config use <provider>`")
		return false
	}
	knownNames := config.ProviderNames()
	known := slices.Contains(knownNames, cfg.SelectedProvider)
	if known {
		pass("selected_provider: " + cfg.SelectedProvider)
	} else {
		fail("selected_provider: " + cfg.SelectedProvider + " (not a known provider)")
		fmt.Printf("    → Supported: %s\n", strings.Join(knownNames, ", "))
	}

	// Provider block.
	pc, hasBlock := cfg.Providers[cfg.SelectedProvider]
	if !hasBlock {
		fail("[providers." + cfg.SelectedProvider + "]: no config block")
		fmt.Println("    → Run `tw config` to configure it")
		return allOk
	}
	pass("[providers." + cfg.SelectedProvider + "]: block found")

	// Model.
	if pc.Model != "" {
		pass("Model: " + pc.Model)
	} else {
		fail("Model: not set")
		fmt.Println("    → Run: tw config model set <model>")
	}

	// Auth credentials.
	if cfg.SelectedProvider == "ollama" {
		pass("Auth: not required for Ollama")
	} else {
		authDesc := authSummary(cfg.SelectedProvider, pc)
		_, authErr := config.ResolveAuth(cfg.SelectedProvider, pc)
		if authErr == nil {
			pass("Auth: " + authDesc + " — key resolved")
		} else {
			fail("Auth: " + authDesc + " — " + authErr.Error())
		}
	}

	// Settings — validated through the central registry so each SettingDef owns its rules.
	for _, def := range config.Settings {
		val := def.Get(cfg)
		display := def.Resolve(cfg)
		label := def.Label
		if def.Validate != nil {
			if err := def.Validate(val); err != nil {
				fail(label + ": " + err.Error())
				continue
			}
		}
		if val == "" {
			pass(label + ": " + display + " (default)")
		} else {
			pass(label + ": " + display)
		}
	}

	fmt.Println()
	if allOk {
		fmt.Println("All checks passed.")
	}
	return allOk
}

// showJSON is the JSON representation of the full effective configuration.
type showJSON struct {
	ConfigPath       string                      `json:"config_path"`
	SelectedProvider string                      `json:"selected_provider"`
	Providers        map[string]showProviderJSON `json:"providers"`
	Settings         map[string]any              `json:"settings"`
}

type showProviderJSON struct {
	Active  bool         `json:"active"`
	Model   string       `json:"model,omitempty"`
	Auth    showAuthJSON `json:"auth"`
	BaseURL string       `json:"base_url,omitempty"`
}

type showAuthJSON struct {
	Method string `json:"method"`
	Source string `json:"source,omitempty"`
}

func printShowJSON(cfgPath string, cfg config.FileConfig) error {
	providers := make(map[string]showProviderJSON, len(cfg.Providers))
	for name, pc := range cfg.Providers {
		providers[name] = showProviderJSON{
			Active:  name == cfg.SelectedProvider,
			Model:   pc.Model,
			Auth:    buildAuthJSON(name, pc),
			BaseURL: pc.BaseURL,
		}
	}

	settings := make(map[string]any, len(config.Settings))
	for _, def := range config.Settings {
		settings[def.Key] = def.GetAny(cfg)
	}

	out := showJSON{
		ConfigPath:       cfgPath,
		SelectedProvider: cfg.SelectedProvider,
		Providers:        providers,
		Settings:         settings,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func buildAuthJSON(name string, pc config.ProviderConfig) showAuthJSON {
	method := pc.Auth
	if method == "" {
		method = "env"
	}
	var source string
	switch method {
	case "env":
		source = pc.EnvVar
		if source == "" {
			source = config.DefaultEnvVar(name)
		}
	case "cmd":
		source = pc.APIKeyCmd
	case "keychain":
		source = pc.KeychainEntry
	}
	return showAuthJSON{Method: method, Source: source}
}

// authSummary returns a compact human-readable auth description.
func authSummary(providerName string, pc config.ProviderConfig) string {
	if providerName == "ollama" {
		return "(no auth)"
	}
	method := pc.Auth
	if method == "" {
		method = "env"
	}
	switch method {
	case "env":
		v := pc.EnvVar
		if v == "" {
			v = config.DefaultEnvVar(providerName)
		}
		return "env (" + v + ")"
	case "cmd":
		v := pc.APIKeyCmd
		if v == "" {
			return "cmd (not set)"
		}
		if len(v) > 40 {
			v = v[:37] + "..."
		}
		return "cmd (" + v + ")"
	case "keychain":
		v := pc.KeychainEntry
		if v == "" {
			return "keychain (not set)"
		}
		return "keychain (" + v + ")"
	default:
		return method
	}
}

// editConfig opens the config file in $EDITOR.
func editConfig() error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := config.SaveConfig(cfgPath, config.FileConfig{}); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		for _, e := range []string{"vim", "nano", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found — set $EDITOR")
	}

	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Validate after editing.
	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
		return nil
	}
	for _, e := range config.Validate(cfg) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", e)
	}
	return nil
}

func init() {
	configCmd.Flags().Bool("edit", false, "Open config file in $EDITOR")

	configShowCmd.Flags().Bool("json", false, "Output as JSON")

	configModelCmd.AddCommand(configModelListCmd)
	configModelCmd.AddCommand(configModelSetCmd)

	configAuthCmd.AddCommand(configAuthEnvCmd)
	configAuthCmd.AddCommand(configAuthShellCmd)
	configAuthCmd.AddCommand(configAuthKeychainCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configUseCmd)
	configCmd.AddCommand(configModelCmd)
	configCmd.AddCommand(configAuthCmd)
	configCmd.AddCommand(configTestCmd)
	configCmd.AddCommand(configDoctorCmd)
	configCmd.AddCommand(configRemoveCmd)
}
