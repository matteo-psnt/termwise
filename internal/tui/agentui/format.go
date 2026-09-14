package agentui

import (
	"errors"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/cliname"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// providerErrMsg returns the error string with contextual advice appended for
// known recoverable error kinds (auth failure, model not found).
func providerErrMsg(err error) string {
	if pe, ok := errors.AsType[*provider.Error](err); ok {
		switch pe.Kind {
		case provider.ErrAuth:
			return pe.Reason + fmt.Sprintf(" Run `%s config` to update your key.", cliname.Name())
		case provider.ErrModelNotFound:
			return pe.Reason + fmt.Sprintf(" Run `%s config` to change the model.", cliname.Name())
		}
	}
	return err.Error()
}
