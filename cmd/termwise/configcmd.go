package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/shellrc"
	"github.com/matteo-psnt/termwise/internal/tty"
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
		if !interactive() {
			return fmt.Errorf("the setup screen needs a terminal — from a script, use `%[1]s config use <provider> [--model …]` (see `%[1]s config use --help`)",
				cliname.Name())
		}
		if err := configui.Open(cfgPath, cfg, exists); err != nil {
			return err
		}
		if !exists && config.ConfigExists(cfgPath) && interactive() {
			offerShellIntegration(os.Stdin, os.Stdout)
		}
		return nil
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
			return fmt.Errorf("%w\n  Run `%s config --edit` to repair the file", err, cliname.Name())
		}
		if !exists {
			if notice := config.EnvOnlyNotice(cfgPath); notice != "" {
				fmt.Fprintln(os.Stderr, notice)
				return nil
			}
			return fmt.Errorf("no configuration file — run `%s config` to set up", cliname.Name())
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
	Use:   "use <provider>",
	Short: "Set the active provider, optionally with its model, auth and address",
	Long: `Set the active provider. A provider that has no config yet gets its
default API key env var and recommended model, so this one command is enough
to get a working setup; the flags change any of those in the same step.`,
	Example: fmt.Sprintf("  %[1]s config use anthropic\n"+
		"  %[1]s config use openai --model gpt-4o --env-var WORK_OPENAI_KEY\n"+
		"  %[1]s config use openrouter --cmd \"op read op://dev/openrouter/key\"\n"+
		"  %[1]s config use ollama --base-url http://gpu-box:11434 --model qwen3:14b", cliname.Name()),
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		model, _ := cmd.Flags().GetString("model")
		envVar, _ := cmd.Flags().GetString("env-var")
		keyCmd, _ := cmd.Flags().GetString("cmd")
		baseURL, _ := cmd.Flags().GetString("base-url")
		if envVar != "" && keyCmd != "" {
			return fmt.Errorf("--env-var and --cmd cannot be used together")
		}
		return mutateConfig(func(cfg *config.FileConfig) error {
			if err := config.UseProvider(cfg, name); err != nil {
				return err
			}
			if model != "" {
				if err := config.SetModel(cfg, name, model); err != nil {
					return err
				}
			}
			if envVar != "" {
				if err := config.SetAuthEnv(cfg, name, envVar); err != nil {
					return err
				}
			}
			if keyCmd != "" {
				if err := config.SetAuthCmd(cfg, name, keyCmd); err != nil {
					return err
				}
			}
			if baseURL != "" {
				return config.SetBaseURL(cfg, name, baseURL)
			}
			return nil
		})
	},
}

// ---------------------------------------------------------------------------
// tw config base-url
// ---------------------------------------------------------------------------

var configBaseURLCmd = &cobra.Command{
	Use:   "base-url <url>",
	Short: "Point the selected provider at a different server",
	Long: `Point the selected provider at a different server: an OpenAI-compatible
proxy such as LiteLLM, a self-hosted model server, or Ollama on another
machine. --reset goes back to the provider's default address.`,
	Example: fmt.Sprintf("  %[1]s config base-url http://gpu-box:11434\n"+
		"  %[1]s config base-url https://litellm.internal/v1\n"+
		"  %[1]s config base-url --reset", cliname.Name()),
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		reset, _ := cmd.Flags().GetBool("reset")
		if reset == (len(args) == 1) {
			return fmt.Errorf("give a URL, or --reset to use the provider's default")
		}
		url := ""
		if len(args) == 1 {
			url = args[0]
		}
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			if err := config.SetBaseURL(cfg, name, url); err != nil {
				return err
			}
			if url != "" && !provider.HasNoAuth(name) {
				fmt.Fprintf(os.Stderr, "Note: your %s API key will be sent to %s.\n", name, url)
			}
			return nil
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
			return fmt.Errorf("no provider selected — pass a provider name or run `%s config use <provider>`", cliname.Name())
		}

		pc, ok := cfg.Providers[name]
		if !ok {
			return fmt.Errorf("provider %q is not configured — run `%s config` to set it up", name, cliname.Name())
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
	Example:      fmt.Sprintf("  %[1]s config model set claude-sonnet-4-6\n  %[1]s config model set gpt-4o", cliname.Name()),
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
				fmt.Fprintf(os.Stderr, "Warning: could not check the model against %s — saved anyway\n  %s\n",
					name, strings.ReplaceAll(err.Error(), "\n", "\n  "))
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
	Example:      fmt.Sprintf("  %[1]s config auth env ANTHROPIC_API_KEY", cliname.Name()),
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
	Example:      fmt.Sprintf("  %[1]s config auth cmd \"op read 'op://personal/openai/api-key'\"", cliname.Name()),
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthCmd(cfg, name, args[0])
		})
	},
}

var configAuthKeychainCmd = &cobra.Command{
	Use:   "keychain [ENTRY]",
	Short: "Use a system keychain entry for auth",
	Long: `Use a system keychain entry (service "termwise") for auth.

--store saves a key first: it is read from stdin, or asked for without echo
when stdin is a terminal, so it never lands in your shell history. With
--store the entry name defaults to termwise-<provider>.`,
	Example: fmt.Sprintf("  %[1]s config auth keychain --store\n"+
		"  op read op://dev/anthropic/key | %[1]s config auth keychain --store\n"+
		"  %[1]s config auth keychain termwise-anthropic", cliname.Name()),
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _ := cmd.Flags().GetBool("store")
		if !store && len(args) == 0 {
			return fmt.Errorf("name the keychain entry, or pass --store to save a key")
		}
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			entry := config.DefaultKeychainEntry(name)
			if len(args) == 1 {
				entry = args[0]
			}
			if store {
				key, err := readSecret(fmt.Sprintf("%s API key: ", name))
				if err != nil {
					return err
				}
				if err := config.StoreKeychainEntry(entry, key); err != nil {
					return fmt.Errorf("saving to keychain: %w", err)
				}
			}
			return config.SetAuthKeychain(cfg, name, entry)
		})
	},
}

// readSecret reads one secret: from a piped stdin as-is, or from a terminal
// without echoing it.
func readSecret(prompt string) (string, error) {
	var raw []byte
	var err error
	if tty.IsTerminal(os.Stdin) {
		fmt.Fprint(os.Stderr, prompt)
		raw, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return "", fmt.Errorf("reading key: %w", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return "", fmt.Errorf("no key given")
	}
	return key, nil
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
			return fmt.Errorf("no provider selected — run `%s config use <provider>`", cliname.Name())
		}
		pc, ok := cfg.Providers[name]
		if !ok {
			return fmt.Errorf("provider %q has no config block — run `%s config` to set it up", name, cliname.Name())
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
	Short:        "Check configuration, provider connection and shell setup",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		offline, _ := cmd.Flags().GetBool("offline")
		if !runDoctor(cfgPath, !offline) {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	configDoctorCmd.Flags().Bool("offline", false, "Skip the check that contacts the provider")
}

// ---------------------------------------------------------------------------
// tw config remove <provider>
// ---------------------------------------------------------------------------

var configRemoveCmd = &cobra.Command{
	Use:          "remove <provider>",
	Short:        "Remove a provider config block",
	Example:      fmt.Sprintf("  %[1]s config remove openai", cliname.Name()),
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
		return fmt.Errorf("%w\n  Config file is invalid — run `%s config --edit` to repair it", err, cliname.Name())
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
			return fmt.Errorf("no provider selected — run `%s config use <provider>` first", cliname.Name())
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
// online adds a live request to the provider. Warnings point at something
// worth knowing but do not fail the run.
func runDoctor(cfgPath string, online bool) bool {
	allOk := true
	pass := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	fail := func(msg string) { fmt.Printf("  ✗ %s\n", msg); allOk = false }
	warn := func(msg string) { fmt.Printf("  ! %s\n", msg) }
	hint := func(format string, a ...any) { fmt.Printf("    → "+format+"\n", a...) }

	fmt.Printf("Config: %s\n\n", cfgPath)

	cfg, exists, parseErr := config.LoadConfig(cfgPath)
	if parseErr != nil {
		fail("File: " + parseErr.Error())
		hint("Run `%s config --edit` to repair", cliname.Name())
		return false
	}
	if exists {
		pass("File: exists and valid TOML")
	} else if zero, ok := config.ZeroConfigDefaults(); ok {
		pass("File: none — running from an API key in the environment")
		hint("Run `%s config` to save a config", cliname.Name())
		cfg = zero
	} else {
		fail("File: not found, and no provider API key in the environment")
		hint("Run `%s config` to set up", cliname.Name())
		return false
	}

	// selected_provider set and known.
	if cfg.SelectedProvider == "" {
		fail("selected_provider: not set")
		fmt.Printf("    → Run `%s config use <provider>`\n", cliname.Name())
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
		fmt.Printf("    → Run `%s config` to configure it\n", cliname.Name())
		return allOk
	}
	pass("[providers." + cfg.SelectedProvider + "]: block found")

	// Model.
	if pc.Model != "" {
		pass("Model: " + pc.Model)
	} else {
		fail("Model: not set")
		hint("Run: %s config model set <model>", cliname.Name())
	}

	// Auth credentials.
	authOk := true
	if provider.HasNoAuth(cfg.SelectedProvider) {
		pass("Auth: not required")
	} else {
		authDesc := authSummary(cfg.SelectedProvider, pc)
		_, authErr := config.ResolveAuth(cfg.SelectedProvider, pc)
		if authErr == nil {
			pass("Auth: " + authDesc + " — key resolved")
		} else {
			fail("Auth: " + authDesc + " — " + authErr.Error())
			authOk = false
		}
	}

	// Connection: the only check that proves the key and URL actually work.
	switch {
	case !online:
		warn("Connection: skipped (--offline)")
	case !known || !authOk:
		warn("Connection: skipped until the problems above are fixed")
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := config.CheckProviderConnectivity(ctx, cfg.SelectedProvider, pc)
		cancel()
		if err == nil {
			pass("Connection: " + cfg.SelectedProvider + " answered")
		} else {
			fail("Connection: " + strings.ReplaceAll(err.Error(), "\n", "\n    "))
		}
	}

	doctorShell(pass, warn, hint, cfg)

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
	method := pc.AuthMethod()
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
	if provider.HasNoAuth(providerName) {
		return "(no auth)"
	}
	switch method := pc.AuthMethod(); method {
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

	configUseCmd.ValidArgs = config.ProviderNames()
	configUseCmd.Flags().String("model", "", "Model to use")
	configUseCmd.Flags().String("env-var", "", "Read the API key from this environment variable")
	configUseCmd.Flags().String("cmd", "", "Run this command to get the API key")
	configUseCmd.Flags().String("base-url", "", "Server address, for proxies and self-hosted servers")

	configBaseURLCmd.Flags().Bool("reset", false, "Use the provider's default address")

	configAuthKeychainCmd.Flags().Bool("store", false, "Save a key to the keychain first")

	configModelCmd.AddCommand(configModelListCmd)
	configModelCmd.AddCommand(configModelSetCmd)

	configAuthCmd.AddCommand(configAuthEnvCmd)
	configAuthCmd.AddCommand(configAuthShellCmd)
	configAuthCmd.AddCommand(configAuthKeychainCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configUseCmd)
	configCmd.AddCommand(configBaseURLCmd)
	configCmd.AddCommand(configModelCmd)
	configCmd.AddCommand(configAuthCmd)
	configCmd.AddCommand(configTestCmd)
	configCmd.AddCommand(configDoctorCmd)
	configCmd.AddCommand(configRemoveCmd)
}

// doctorShell checks the pieces outside the config file that `tw` and the
// keybinding depend on. Everything here is a warning: termwise itself works
// without the integration.
func doctorShell(pass, warn func(string), hint func(string, ...any), cfg config.FileConfig) {
	if _, err := exec.LookPath("termwise"); err == nil {
		pass("PATH: termwise found")
	} else {
		warn("PATH: termwise is not on your PATH, so `tw` and the shell integration cannot run it")
		if exe, err := os.Executable(); err == nil {
			hint("Add %s to PATH in your shell's startup file", config.ShortenHome(filepath.Dir(exe)))
		}
	}

	shell, ok := shellrc.Detect()
	loaded := os.Getenv(cliname.EnvVar) != ""
	switch {
	case loaded:
		pass("Shell integration: loaded in this shell")
	case !ok:
		warn("Shell integration: not loaded, and $SHELL is not zsh, bash or fish")
	default:
		path, err := shellrc.Path(shell)
		if err != nil {
			break
		}
		if installed, _ := shellrc.Installed(path); installed {
			warn("Shell integration: in " + config.ShortenHome(path) + " but not loaded in this shell")
			hint("Open a new terminal, or run `exec %s`", shell)
		} else {
			warn("Shell integration: not installed — no `tw` command or keybinding")
			hint("Run `termwise init --install`")
		}
	}

	if kb := cfg.Settings.Keybinding; kb == "" || kb == config.DefaultKeybinding {
		if _, err := exec.LookPath("fzf"); err == nil {
			warn("Keybinding: fzf is installed, and its shell keybindings also use Ctrl+T — whichever loads last wins")
			hint("Pick another key in `%s config` → Keybinding", cliname.Name())
		}
	}
}
