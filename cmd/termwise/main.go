package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/runner"
	"github.com/matteo-psnt/termwise/internal/tui/agentui"
)

func main() {
	root := &cobra.Command{
		Use:   "tw [prompt]",
		Short: "Terminal AI assistant",
		Long:  "tw — pass a prompt for single-shot mode, or run without arguments to open the agent TUI.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --shell-widget is the legacy Ctrl+T text-transform path (kept for compatibility).
			if cmd.Flags().Changed("shell-widget") {
				sw, _ := cmd.Flags().GetString("shell-widget")
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				return runner.ShellWidget(ctx, sw)
			}
			prefill, _ := cmd.Flags().GetString("prefill")
			if len(args) == 0 {
				return openTUI(prefill)
			}
			return runner.SingleShot(context.Background(), strings.Join(args, " "))
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().String("shell-widget", "", "")
	root.Flags().Lookup("shell-widget").Hidden = true
	root.Flags().String("prefill", "", "")
	root.Flags().Lookup("prefill").Hidden = true

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

func openTUI(prefill string) error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	rc, err := config.LoadRuntimeConfig(cfgPath)
	if err != nil {
		return err
	}
	client, err := config.NewClientFromResolved(rc)
	if err != nil {
		return err
	}
	return agentui.Open(rc.ProviderName, client, rc.Model, cfgPath, prefill)
}
