package configui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
)

// authInput is the text field for a provider's credential setting (env var
// name, shell command, API key or server address), shared by the wizard and
// the auth editor. It embeds the textinput, so Focus, Blur and Value are the
// textinput's own.
type authInput struct {
	textinput.Model
	label    string
	hint     string
	fallback string // used when the field is left blank; empty means a value is required
}

func newAuthInput() authInput {
	ti := textinput.New()
	ti.CharLimit = 256
	return authInput{Model: ti}
}

// setup configures the field for providerName's method from
// buildAuthInputConfig, fills it with initial and focuses it.
func (a *authInput) setup(providerName, method, initial string) {
	cfg := buildAuthInputConfig(providerName, method)
	a.label = cfg.Label
	a.hint = cfg.Hint
	a.fallback = cfg.Fallback
	a.Placeholder = cfg.Placeholder
	a.EchoMode = cfg.EchoMode
	a.SetValue(initial)
	a.Focus()
}

// errEmptyValue is shown when enter is pressed on a field that is missing.
const errEmptyValue = "value cannot be empty"

// missing reports whether the field is blank with no fallback to stand in.
func (a authInput) missing() bool {
	return strings.TrimSpace(a.Value()) == "" && a.fallback == ""
}

// view renders the label, field and hint, then errMsg if set, then help.
func (a authInput) view(styles configStyles, errMsg, help string) string {
	var b strings.Builder
	b.WriteString(a.label + ":\n\n")
	b.WriteString(a.View() + "\n")
	if a.hint != "" {
		b.WriteString("\n" + styles.Hint.Render(a.hint))
	}
	if errMsg != "" {
		b.WriteString("\n\n" + styles.Error.Render("Error: "+errMsg))
	}
	b.WriteString("\n\n" + styles.Dim.Render(help))
	return b.String()
}
