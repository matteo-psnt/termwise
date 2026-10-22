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

func TestMapError(t *testing.T) {
	if MapError(nil, testStatus) != nil {
		t.Fatal("nil error should map to nil")
	}
	if err := MapError(fmt.Errorf("wrap: %w", context.Canceled), testStatus); !errors.Is(err, context.Canceled) {
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
		{"server error", &statusErr{500, "boom"}, ErrProvider, "boom"},
		{"transport", errors.New("dial tcp: refused"), ErrProvider, "dial tcp: refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pe *Error
			if !errors.As(MapError(tt.err, testStatus), &pe) {
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
