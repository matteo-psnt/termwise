package configui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

var authMethods = []struct {
	id    string
	label string
}{
	{"env", "Environment variable"},
	{"keychain", "macOS Keychain"},
	{"cmd", "Shell command"},
}

type authInputConfig struct {
	Label       string
	Hint        string
	Fallback    string
	Placeholder string
	EchoMode    textinput.EchoMode
}

// buildAuthInputConfig returns the label/hint/fallback/placeholder for a given
// provider+method combination. Providers with NoAuth=true take a base URL
// instead of credentials.
func buildAuthInputConfig(providerName, method string) authInputConfig {
	if provider.HasNoAuth(providerName) {
		return authInputConfig{
			Label:       "Base URL",
			Hint:        "leave blank for default (" + defaultOllamaBaseURL + ")",
			Fallback:    defaultOllamaBaseURL,
			Placeholder: defaultOllamaBaseURL,
		}
	}
	switch method {
	case "env":
		def := config.DefaultEnvVar(providerName)
		return authInputConfig{
			Label:       "Environment variable name",
			Hint:        "the env var that holds your API key",
			Fallback:    def,
			Placeholder: def,
		}
	case "cmd":
		return authInputConfig{
			Label:       "Shell command",
			Hint:        "command whose stdout is the API key",
			Placeholder: "op read op://vault/item/field",
		}
	case "keychain":
		return authInputConfig{
			Label:       "API key",
			Hint:        "will be stored securely in macOS Keychain",
			Placeholder: "sk-...",
			EchoMode:    textinput.EchoPassword,
		}
	}
	return authInputConfig{}
}

// envVarDetected reports whether the default env var for provider is set.
func envVarDetected(providerName string) bool {
	defVar := config.DefaultEnvVar(providerName)
	return defVar != "" && os.Getenv(defVar) != ""
}

// renderAuthMethodRows renders the auth method picker list.
// currentMethod: show "(current)" badge on the matching row.
// showDetected: show ● next to env if the default env var is set.
func renderAuthMethodRows(providerName string, cursor int, currentMethod string, showDetected bool, styles configStyles) string {
	var b strings.Builder
	for i, a := range authMethods {
		badges := ""
		if a.id == currentMethod {
			badges += " " + styles.Dim.Render("(current)")
		}
		if showDetected && a.id == "env" && envVarDetected(providerName) {
			badges += "  " + styles.Success.Render("●")
		}
		if i == cursor {
			b.WriteString(styles.Selected.Render("▶ "+a.label) + badges + "\n")
		} else {
			b.WriteString(styles.Normal.Render("  "+a.label) + badges + "\n")
		}
	}
	return b.String()
}
