package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// providerTimeout bounds every request these commands make to a provider, so
// an unreachable server fails the command instead of hanging it.
const providerTimeout = 15 * time.Second

// selectedProviderConfig returns the config block for name, or for the
// selected provider when name is empty.
func selectedProviderConfig(cfg config.FileConfig, name string) (string, config.ProviderConfig, error) {
	if name == "" {
		name = cfg.SelectedProvider
	}
	if name == "" {
		return "", config.ProviderConfig{}, fmt.Errorf("no provider selected — run `%s config use <provider>`", cliname.Name())
	}
	pc, ok := cfg.Providers[name]
	if !ok {
		return "", config.ProviderConfig{}, fmt.Errorf("provider %q is not configured — run `%s config` to set it up", name, cliname.Name())
	}
	return name, pc, nil
}

// showStatus prints msg on stderr as a transient status line and returns a
// func that erases it, so the wait is visible but leaves nothing behind in
// the output.
func showStatus(msg string) func() {
	fmt.Fprint(os.Stderr, msg)
	return func() { fmt.Fprint(os.Stderr, "\r\033[K") }
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
		_, cfg, _, err := loadConfigFile()
		if err != nil {
			return err
		}
		var name string
		if len(args) == 1 {
			name = args[0]
		}
		name, pc, err := selectedProviderConfig(cfg, name)
		if err != nil {
			return err
		}

		done := showStatus(fmt.Sprintf("Fetching models for %s...", name))
		ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
		defer cancel()

		models, err := config.ListProviderModels(ctx, name, pc)
		done()
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
			ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
			defer cancel()

			done := showStatus("Validating model...")
			models, err := config.ListProviderModels(ctx, name, pc)
			done()
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
// tw config test
// ---------------------------------------------------------------------------

var configTestCmd = &cobra.Command{
	Use:          "test",
	Short:        "Test connectivity for the selected provider",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		_, cfg, _, err := loadConfigFile()
		if err != nil {
			return err
		}
		name, pc, err := selectedProviderConfig(cfg, "")
		if err != nil {
			return err
		}

		done := showStatus(fmt.Sprintf("Testing %s...", name))
		ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
		defer cancel()

		err = config.CheckProviderConnectivity(ctx, name, pc)
		done()
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		fmt.Printf("✓ %s is reachable\n", name)
		return nil
	},
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

func init() {
	configUseCmd.ValidArgs = provider.Names()
	configUseCmd.Flags().String("model", "", "Model to use")
	configUseCmd.Flags().String("env-var", "", "Read the API key from this environment variable")
	configUseCmd.Flags().String("cmd", "", "Run this command to get the API key")
	configUseCmd.Flags().String("base-url", "", "Server address, for proxies and self-hosted servers")

	configBaseURLCmd.Flags().Bool("reset", false, "Use the provider's default address")
}
