package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/runner"
)

var askCmd = &cobra.Command{
	Use:          "ask <prompt>",
	Short:        "Run the headless agent path and print only the final response",
	Example:      "  tw ask \"undo last commit but keep changes\"\n  tw ask \"what port is my dev server using\"",
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runner.Ask(context.Background(), strings.Join(args, " "))
	},
}
