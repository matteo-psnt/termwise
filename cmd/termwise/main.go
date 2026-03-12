package main

import (
	"context"
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
			if len(args) == 0 {
				return openTUI()
			}
			prompt := strings.Join(args, " ")
			return runner.SingleShot(context.Background(), prompt)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tw: %s\n", err)
		os.Exit(1)
	}
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
	return tui.Open(providerName, provider, pc.Model)
}
