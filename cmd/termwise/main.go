package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	// Register providers via their init() functions.
	_ "github.com/matteo-psnt/termwise/internal/ai/anthropic"
	_ "github.com/matteo-psnt/termwise/internal/ai/openaicompat"

	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/runner"
	"github.com/matteo-psnt/termwise/internal/tui"
)

func main() {
	root := &cobra.Command{
		Use:   "tw [prompt]",
		Short: "Terminal AI assistant",
		Long:  "tw — pass a prompt for single-shot mode, or run without arguments to open the agent TUI.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --shell-widget is the internal Ctrl+T keybinding path.
			if sw, _ := cmd.Flags().GetString("shell-widget"); sw != "" {
				return runner.ShellWidget(context.Background(), sw)
			}
			if len(args) == 0 {
				return openTUI()
			}
			return runner.SingleShot(context.Background(), strings.Join(args, " "))
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().String("shell-widget", "", "")
	root.Flags().Lookup("shell-widget").Hidden = true

	root.AddCommand(initCmd)
	root.AddCommand(configCmd)

	if err := root.Execute(); err != nil {
		// ExitCode is a sentinel — use the embedded code, print nothing.
		var ec runner.ExitCode
		if errors.As(err, &ec) {
			os.Exit(ec.Code)
		}
		fmt.Fprintf(os.Stderr, "tw: %s\n", err)
		os.Exit(1)
	}

	// Check if last command returned an ExitCode through normal return path.
	// (cobra swallows non-nil errors; RunE errors land in the Execute() return above.)
}

func openTUI() error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, exists, err := config.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	if !exists {
		var ok bool
		cfg, ok = config.ZeroConfigDefaults()
		if !ok {
			return fmt.Errorf("no configuration found — run `tw config` to set up")
		}
	}
	if err := config.ValidateForRuntime(cfg); err != nil {
		return err
	}
	providerName, pc, err := cfg.ActiveProviderConfig()
	if err != nil {
		return err
	}
	auth, err := config.ResolveAuth(providerName, pc)
	if err != nil {
		return err
	}
	provider, err := ai.GetProvider(providerName, ai.ProviderConfig{
		APIKey:  auth.APIKey,
		Model:   pc.Model,
		BaseURL: auth.BaseURL,
	})
	if err != nil {
		return err
	}
	return tui.Open(providerName, provider, pc.Model, cfgPath, cfg.Shell.Allow)
}
