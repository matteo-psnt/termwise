package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// ---------------------------------------------------------------------------
// tw config show
// ---------------------------------------------------------------------------

var configShowCmd = &cobra.Command{
	Use:          "show",
	Short:        "Print the effective configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		// Load the file first so a broken one gets the repair hint, then
		// report what the runtime would run with: the zero-config fallback
		// when there is no file, and TERMWISE_* overrides on top.
		cfgPath, _, err := loadConfigFile()
		if err != nil {
			return err
		}
		eff, err := config.LoadEffective(cfgPath)
		if errors.Is(err, config.ErrNoConfig) {
			return fmt.Errorf("no configuration file — run `%s config` to set up", cliname.Name())
		}
		if err != nil {
			return err
		}
		source := sourceFile
		if !eff.Exists {
			// --json callers still get a document to parse, and can tell
			// it apart by its source.
			source = sourceEnvironment
			fmt.Fprintln(os.Stderr, config.EnvOnlyNotice(cfgPath))
		}
		if asJSON {
			return printShowJSON(cfgPath, source, eff)
		}
		return printShow(cfgPath, source, eff)
	},
}

func init() {
	configShowCmd.Flags().Bool("json", false, "Output as JSON")
}

// Where the config `show` prints came from.
const (
	sourceFile        = "file"
	sourceEnvironment = "environment"
)

// printShow writes a human-readable view of the full configuration to stdout.
func printShow(cfgPath, source string, eff config.Effective) error {
	cfg := eff.File
	ov := overrideMap(eff)
	if source == sourceEnvironment {
		fmt.Printf("Config: none at %s — from the environment\n", cfgPath)
	} else {
		fmt.Printf("Config: %s\n", cfgPath)
	}
	for _, o := range eff.Overrides {
		fmt.Printf("Override: %s (from the environment, not the file)\n", o)
	}

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
				if _, ok := ov[config.EnvProvider]; ok {
					label += " [" + config.EnvProvider + "]"
				}
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
			if _, ok := ov[config.EnvModel]; ok && active {
				model += " [" + config.EnvModel + "]"
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

// showJSON is the JSON representation of the full effective configuration.
type showJSON struct {
	ConfigPath string `json:"config_path"`
	// Source is "file", or "environment" when there is no file at
	// ConfigPath and the config comes from an API key env var.
	Source           string                      `json:"source"`
	SelectedProvider string                      `json:"selected_provider"`
	Providers        map[string]showProviderJSON `json:"providers"`
	Settings         map[string]any              `json:"settings"`
	// Overrides maps each TERMWISE_* variable in effect to its value; the
	// fields above already include them.
	Overrides map[string]string `json:"overrides,omitempty"`
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

func printShowJSON(cfgPath, source string, eff config.Effective) error {
	cfg := eff.File
	providers := make(map[string]showProviderJSON, len(cfg.Providers))
	for name, pc := range cfg.Providers {
		providers[name] = showProviderJSON{
			Active:  name == cfg.SelectedProvider,
			Model:   pc.Model,
			Auth:    authJSON(name, pc),
			BaseURL: pc.BaseURL,
		}
	}

	settings := make(map[string]any, len(config.Settings))
	for _, def := range config.Settings {
		settings[def.Key] = def.GetAny(cfg)
	}

	out := showJSON{
		ConfigPath:       cfgPath,
		Source:           source,
		SelectedProvider: cfg.SelectedProvider,
		Providers:        providers,
		Settings:         settings,
		Overrides:        overrideMap(eff),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// overrideMap returns the TERMWISE_* overrides in effect by variable name,
// or nil when there are none.
func overrideMap(eff config.Effective) map[string]string {
	if len(eff.Overrides) == 0 {
		return nil
	}
	m := make(map[string]string, len(eff.Overrides))
	for _, o := range eff.Overrides {
		name, val, _ := strings.Cut(o, "=")
		m[name] = val
	}
	return m
}

func authJSON(name string, pc config.ProviderConfig) showAuthJSON {
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
		return "cmd (" + truncateRunes(v, 40) + ")"
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

// truncateRunes shortens s to at most n runes, ending in "..." when cut. It
// counts runes, not bytes, so a command with non-ASCII text is never cut
// mid-character into invalid UTF-8.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
