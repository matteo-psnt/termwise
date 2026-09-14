package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/tui/agentui"
)

var explainCmd = &cobra.Command{
	Use:   "explain [command]",
	Short: "Explain a shell command",
	Long: `Explain what a shell command does, flag by flag.

With a command, the TUI opens and explains it immediately. Without one, the TUI
opens and asks which command to explain. Either way the agent only explains —
it is given no tool to propose a command of its own.`,
	Example: "  tw explain 'awk -F: \"{print \\$1}\" /etc/passwd'\n  tw explain\n  fc -ln -1 | tw explain",
	Args:    cobra.ArbitraryArgs,
	// Explain always opens the TUI, so warnings go inline rather than to the
	// stderr the alt-screen is about to wipe.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID, _ := cmd.Flags().GetString("session-id")
		resume, _ := cmd.Flags().GetBool("resume")

		rt, err := resolveTUIRuntime()
		if err != nil {
			return err
		}
		return agentui.OpenExplain(rt.providerName, rt.client, rt.modelID, rt.cfgPath,
			explainPrompt(args), sessionID, resume)
	},
}

// explainPrompt turns the positional arguments into the prompt to submit.
// Empty args mean the user wants the input box, so it returns "" and the TUI
// opens without submitting anything.
func explainPrompt(args []string) string {
	joined := strings.TrimSpace(strings.Join(args, " "))
	if joined == "" {
		return ""
	}
	return "Explain this command:\n\n" + joined
}
