package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	_ "github.com/matteo-psnt/termwise/internal/provider/builtin"
	"github.com/matteo-psnt/termwise/internal/tui/agentui"
)

func main() {
	root := &cobra.Command{
		Use:   "tw [prompt]",
		Short: "Terminal AI assistant",
		Long:  "tw — open the agent TUI, optionally with an initial prompt. Use `tw ask` for a headless agent response.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, _ := cmd.Flags().GetString("session-id")
			resume, _ := cmd.Flags().GetBool("resume")
			shellWidget, _ := cmd.Flags().GetBool("shell-widget")
			if shellWidget {
				return runShellWidget(sessionID, resume, args)
			}

			if len(args) == 0 {
				return openTUI(sessionID, resume)
			}
			return openTUIWithPrompt(strings.Join(args, " "), sessionID, resume)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().String("session-id", "", "")
	root.Flags().Lookup("session-id").Hidden = true
	root.Flags().Bool("shell-widget", false, "")
	root.Flags().Lookup("shell-widget").Hidden = true
	root.Flags().Bool("resume", false, "Resume the most recent session for this terminal")

	root.AddCommand(askCmd)
	root.AddCommand(initCmd)
	root.AddCommand(configCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tw: %s\n", err)
		os.Exit(1)
	}
}

func openTUI(sessionID string, forceResume bool) error {
	rt, err := resolveTUIRuntime()
	if err != nil {
		return err
	}
	return agentui.Open(rt.providerName, rt.client, rt.modelID, rt.cfgPath, sessionID, forceResume)
}

func openTUIWithPrompt(initialPrompt string, sessionID string, forceResume bool) error {
	rt, err := resolveTUIRuntime()
	if err != nil {
		return err
	}
	return agentui.OpenWithPrompt(rt.providerName, rt.client, rt.modelID, rt.cfgPath, initialPrompt, sessionID, forceResume)
}

type tuiRuntime struct {
	cfgPath      string
	providerName string
	client       provider.AgentClient
	modelID      string
}

func resolveTUIRuntime() (tuiRuntime, error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return tuiRuntime{}, err
	}
	// TUI surfaces config warnings inline (the alt-screen wipes stderr).
	// Non-TUI commands (ask, config) still emit them to stderr.
	rc, err := config.LoadRuntimeConfig(cfgPath)
	if err != nil {
		return tuiRuntime{}, err
	}
	client, err := config.NewClientFromResolved(rc)
	if err != nil {
		return tuiRuntime{}, err
	}
	return tuiRuntime{
		cfgPath:      cfgPath,
		providerName: rc.ProviderName,
		client:       client,
		modelID:      rc.Model,
	}, nil
}

// emitConfigWarnings prints any non-fatal config warnings to stderr. Safe to
// call before the TUI takes over; warnings land in the user's terminal
// scrollback (or stderr for non-interactive commands).
func emitConfigWarnings(cfgPath string) {
	for _, w := range config.CheckConfigWarnings(cfgPath) {
		fmt.Fprintln(os.Stderr, "tw: warning:", w)
	}
}

func runShellWidget(sessionID string, forceResume bool, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("shell widget mode does not accept positional arguments")
	}
	rt, err := resolveTUIRuntime()
	if err != nil {
		return err
	}
	result, err := agentui.OpenShellWidget(rt.providerName, rt.client, rt.modelID, rt.cfgPath, sessionID, forceResume)
	if err != nil {
		return err
	}
	fmt.Print(result)
	return nil
}
