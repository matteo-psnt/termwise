//go:build !unix

package tty

import (
	"errors"
	"time"
)

// waitReadable cannot bound a terminal read here, so it refuses rather than
// risk blocking startup on a terminal that never answers.
func waitReadable(int, time.Duration) error {
	return errors.New("cursor query unsupported on this platform")
}
