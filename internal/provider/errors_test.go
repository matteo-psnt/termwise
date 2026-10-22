package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type statusErr struct {
	code int
	msg  string
}

func (e *statusErr) Error() string { return e.msg }

func testStatus(err error) (int, bool) {
	var se *statusErr
	if errors.As(err, &se) {
		return se.code, true
	}
	return 0, false
}

// timeoutErr is a net.Error that reports a timeout without wrapping
// context.DeadlineExceeded, as a stalled dial or TLS handshake does.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestMapError(t *testing.T) {
	if MapError("Acme", nil, testStatus) != nil {
		t.Fatal("nil error should map to nil")
	}
	if err := MapError("Acme", fmt.Errorf("wrap: %w", context.Canceled), testStatus); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation should pass through, got %v", err)
	}

	tests := []struct {
		name   string
		err    error
		kind   ErrorKind
		reason string
	}{
		{"deadline", context.DeadlineExceeded, ErrTimeout, "request timed out"},
		{"auth", &statusErr{401, "x"}, ErrAuth, "API key invalid or expired."},
		{"forbidden", &statusErr{403, "x"}, ErrAuth, "API key invalid or expired."},
		{"rate limit", &statusErr{429, "x"}, ErrRateLimit, "Rate limited. Try again in a moment."},
		{"gateway timeout", &statusErr{504, "x"}, ErrTimeout, "Request timed out."},
		{"not found", &statusErr{404, "x"}, ErrModelNotFound, "Model not found."},
		{"too long", &statusErr{400, "prompt is too long"}, ErrInputTooLarge, "Input too large for this model. Try piping less data."},
		{"bad request", &statusErr{400, "bad field"}, ErrProvider, "bad field"},
		{"server error", &statusErr{500, "boom"}, ErrServer, "Acme had a server error (HTTP 500) — try again."},
		{"bad gateway", &statusErr{502, "<html>"}, ErrServer, "Acme had a server error (HTTP 502) — try again."},
		{"overloaded", &statusErr{529, `{"type":"error","error":{"type":"overloaded_error"}}`}, ErrServer, "Acme is overloaded or unavailable right now — try again in a moment."},
		{"unavailable", &statusErr{503, "x"}, ErrServer, "Acme is overloaded or unavailable right now — try again in a moment."},
		{"request timeout", &statusErr{408, "x"}, ErrTimeout, "Request timed out."},
		{"net timeout", fmt.Errorf("post: %w", timeoutErr{}), ErrTimeout, "request timed out"},
		{"transport", errors.New("dial tcp: refused"), ErrProvider, "dial tcp: refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pe *Error
			if !errors.As(MapError("Acme", tt.err, testStatus), &pe) {
				t.Fatal("expected *Error")
			}
			if pe.Kind != tt.kind || pe.Reason != tt.reason {
				t.Fatalf("got (%v, %q), want (%v, %q)", pe.Kind, pe.Reason, tt.kind, tt.reason)
			}
			if !errors.Is(pe, tt.err) {
				t.Fatal("underlying error should be wrapped")
			}
		})
	}
}
