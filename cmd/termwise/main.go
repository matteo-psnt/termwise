package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/runner"
	"github.com/matteo-psnt/termwise/internal/tui/agentui"
)

func main() {
	root := &cobra.Command{
		Use:   "tw [prompt]",
		Short: "Terminal AI assistant",
		Long:  "tw — open the agent TUI, optionally with an initial prompt. Use `tw ask` for a headless one-shot response.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			prefill, _ := cmd.Flags().GetString("prefill")
			sessionID, _ := cmd.Flags().GetString("session-id")
			resume, _ := cmd.Flags().GetBool("resume")
			if len(args) == 0 {
				return openTUI(prefill, "", sessionID, resume)
			}
			return openTUI("", strings.Join(args, " "), sessionID, resume)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().String("prefill", "", "")
	root.Flags().Lookup("prefill").Hidden = true
	root.Flags().String("session-id", "", "")
	root.Flags().Lookup("session-id").Hidden = true
	root.Flags().Bool("resume", false, "Resume the most recent session for this terminal")

	root.AddCommand(askCmd)
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

func openTUI(initialDraft string, initialPrompt string, sessionID string, forceResume bool) error {
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
	return agentui.Open(rc.ProviderName, client, rc.Model, cfgPath, initialDraft, initialPrompt, sessionID, forceResume)
}
