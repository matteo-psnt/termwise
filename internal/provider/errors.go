package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ErrorKind categorises provider errors for the CLI/TUI layer.
type ErrorKind int

const (
	ErrAuth ErrorKind = iota
	ErrRateLimit
	ErrTimeout
	ErrModelNotFound
	ErrInputTooLarge
	ErrProvider // catch-all
	// ErrServer is a server-side failure (5xx, including 529 overloaded): the request
	// itself was fine and the same one may succeed if retried later.
	ErrServer
)

// Error carries a user-facing reason and an underlying technical error.
type Error struct {
	Kind   ErrorKind
	Reason string // shown to the user
	Err    error  // underlying detail for debugging
}

func (e *Error) Error() string { return e.Reason }
func (e *Error) Unwrap() error { return e.Err }

// NewError is a convenience constructor used by client packages.
func NewError(kind ErrorKind, reason string, err error) *Error {
	return &Error{Kind: kind, Reason: reason, Err: err}
}

// MapError converts an SDK error into an *Error. The SDKs differ only in the
// type they use for HTTP errors, so each client passes a status func that
// unwraps its own type (with errors.As) and reports the HTTP status code.
// Everything else — cancellation, deadlines, transport failures — is handled
// the same way for every provider. name is the provider's display name, used
// in reasons that tell the user whose side the problem is on.
func MapError(name string, err error, status func(error) (int, bool)) error {
	if err == nil {
		return nil
	}

	// Context cancellation (ctrl+c) is not a provider failure; pass it
	// through untouched so callers can recognise it with errors.Is.
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(ErrTimeout, "request timed out", err)
	}

	if code, ok := status(err); ok {
		return FromHTTPStatus(name, code, err)
	}

	// A transport-level timeout (e.g. a stalled TLS handshake) does not
	// always wrap DeadlineExceeded, but it is still a timeout, not a fault.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return NewError(ErrTimeout, "request timed out", err)
	}

	return NewError(ErrProvider, err.Error(), err)
}

// statusOverloaded is Anthropic's non-standard "overloaded" status.
const statusOverloaded = 529

// FromHTTPStatus classifies an API error by its HTTP status code. err is the
// SDK's error; its message is the reason for anything without a friendlier
// one, and is also searched to tell an oversized input apart from other 400s.
// name is the provider's display name.
func FromHTTPStatus(name string, code int, err error) *Error {
	body := err.Error()
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return NewError(ErrAuth,
			"API key invalid or expired.", err)

	case http.StatusTooManyRequests:
		return NewError(ErrRateLimit,
			"Rate limited. Try again in a moment.", err)

	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return NewError(ErrTimeout,
			"Request timed out.", err)

	case http.StatusNotFound:
		return NewError(ErrModelNotFound,
			"Model not found.", err)

	case http.StatusBadRequest:
		if strings.Contains(body, "too large") || strings.Contains(body, "too long") {
			return NewError(ErrInputTooLarge,
				"Input too large for this model. Try piping less data.", err)
		}
		return NewError(ErrProvider, body, err)

	case statusOverloaded, http.StatusServiceUnavailable:
		// The raw body here is an opaque JSON blob that tells the user
		// nothing more; it stays reachable through Err for debugging.
		return NewError(ErrServer,
			name+" is overloaded or unavailable right now — try again in a moment.", err)

	default:
		if code >= http.StatusInternalServerError {
			return NewError(ErrServer,
				fmt.Sprintf("%s had a server error (HTTP %d) — try again.", name, code), err)
		}
		return NewError(ErrProvider, body, err)
	}
}
