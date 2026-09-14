package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	_ "github.com/matteo-psnt/termwise/internal/provider/builtin"
	"github.com/matteo-psnt/termwise/internal/tui/agentui"
)

// Injected at build time by goreleaser's ldflags. The defaults are what a
// `go install` or a local `go build` reports, which is all either can honestly
// say about where the binary came from.
var (
	version = "dev"
	commit  = "none"
)

// versionString renders what `tw --version` prints. The commit is what makes a
// bug report from a dev build actionable, so it is included whenever there is
// one to show.
func versionString() string {
	if commit == "none" {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}

func main() {
	root := &cobra.Command{
		Use:   cliname.Name() + " [prompt]",
		Short: "Terminal AI assistant",
		Long: fmt.Sprintf("%[1]s — open the agent TUI, optionally with an initial prompt. "+
			"Use `%[1]s ask` for a headless agent response.", cliname.Name()),
		// Cobra turns this into --version for free.
		Version: versionString(),
		Args:    cobra.ArbitraryArgs,
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

	// session-id and resume are persistent so TUI subcommands (explain) can take
	// them after the subcommand name, which is the only position the shell
	// wrapper can put them in. shell-widget stays root-local — it has no meaning
	// anywhere else.
	root.PersistentFlags().String("session-id", "", "")
	root.PersistentFlags().Lookup("session-id").Hidden = true
	root.PersistentFlags().Bool("resume", false, "Resume the most recent session for this terminal")
	root.Flags().Bool("shell-widget", false, "")
	root.Flags().Lookup("shell-widget").Hidden = true

	root.AddCommand(askCmd)
	root.AddCommand(explainCmd)
	root.AddCommand(initCmd)
	root.AddCommand(configCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", cliname.Name(), err)
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
		fmt.Fprintf(os.Stderr, "%s: warning: %s\n", cliname.Name(), w)
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
