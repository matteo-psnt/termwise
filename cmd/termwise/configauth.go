package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// ---------------------------------------------------------------------------
// tw config auth
// ---------------------------------------------------------------------------

var configAuthCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Configure authentication for the selected provider",
	SilenceUsage: true,
}

var configAuthEnvCmd = &cobra.Command{
	Use:          "env <ENV_VAR>",
	Short:        "Use an environment variable for auth",
	Example:      fmt.Sprintf("  %[1]s config auth env ANTHROPIC_API_KEY", cliname.Name()),
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthEnv(cfg, name, args[0])
		})
	},
}

var configAuthCmdCmd = &cobra.Command{
	Use:          "cmd <COMMAND>",
	Short:        "Use a shell command to retrieve the API key",
	Example:      fmt.Sprintf("  %[1]s config auth cmd \"op read 'op://personal/openai/api-key'\"", cliname.Name()),
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			return config.SetAuthCmd(cfg, name, args[0])
		})
	},
}

var configAuthKeychainCmd = &cobra.Command{
	Use:   "keychain [ENTRY]",
	Short: "Use a system keychain entry for auth",
	Long: `Use a system keychain entry (service "termwise") for auth.

--store saves a key first: it is read from stdin, or asked for without echo
when stdin is a terminal, so it never lands in your shell history. With
--store the entry name defaults to termwise-<provider>.`,
	Example: fmt.Sprintf("  %[1]s config auth keychain --store\n"+
		"  op read op://dev/anthropic/key | %[1]s config auth keychain --store\n"+
		"  %[1]s config auth keychain termwise-anthropic", cliname.Name()),
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _ := cmd.Flags().GetBool("store")
		if !store && len(args) == 0 {
			return fmt.Errorf("name the keychain entry, or pass --store to save a key")
		}
		return mutateActiveProvider(func(cfg *config.FileConfig, name string) error {
			entry := config.DefaultKeychainEntry(name)
			if len(args) == 1 {
				entry = args[0]
			}
			if store {
				key, err := tty.ReadSecret(fmt.Sprintf("%s API key: ", name))
				if err != nil {
					return err
				}
				if err := config.StoreKeychainEntry(entry, key); err != nil {
					return fmt.Errorf("saving to keychain: %w", err)
				}
			}
			return config.SetAuthKeychain(cfg, name, entry)
		})
	},
}

func init() {
	configAuthKeychainCmd.Flags().Bool("store", false, "Save a key to the keychain first")
}
