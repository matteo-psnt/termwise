package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// SingleShot runs a single-shot prompt and writes the response to stdout.
// If stdin is piped, its contents are appended to the prompt automatically.
func SingleShot(ctx context.Context, prompt string) error {
	// Append piped stdin to the prompt.
	if !tty.IsTerminal(os.Stdin) {
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		if len(stdin) > 0 {
			prompt = prompt + "\n\n" + strings.TrimRight(string(stdin), "\n")
		}
	}

	// Load config; fall back to zero-config if no file exists.
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

	resp, err := provider.Complete(ctx, ai.CompleteRequest{
		Model:  pc.Model,
		Prompt: prompt,
	})
	if err != nil {
		return err
	}

	return writeOutput(resp.Content)
}

func writeOutput(content string) error {
	if !tty.IsTerminal(os.Stdout) {
		_, err := fmt.Println(content)
		return err
	}
	rendered, err := glamour.Render(content, "auto")
	if err != nil {
		// Fall back to plain text if rendering fails.
		_, err = fmt.Println(content)
		return err
	}
	_, err = fmt.Print(rendered)
	return err
}
