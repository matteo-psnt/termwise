package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
		cfgPath, cfg, exists, err := loadConfigFile()
		if err != nil {
			return err
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

func init() {
	configShowCmd.Flags().Bool("json", false, "Output as JSON")
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
		SelectedProvider: cfg.SelectedProvider,
		Providers:        providers,
		Settings:         settings,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
