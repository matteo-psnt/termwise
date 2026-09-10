package history

import (
	"strings"
	"testing"
)

func TestExchangeLogRoundTrip(t *testing.T) {
	log := NewExchangeLog(t.TempDir())
	in := []Exchange{
		{Prompt: "install gemini", Kind: ExchangeKindCommand, Response: "brew install gemini-cli", Tools: []string{"bash", "command"}, Turns: 2},
		{Prompt: "what port is 5173", Kind: ExchangeKindText, Response: "vite dev server"},
	}
	for _, e := range in {
		if err := log.Append(e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got, err := log.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d exchanges, want 2", len(got))
	}
	if got[0].Response != "brew install gemini-cli" || got[0].Turns != 2 {
		t.Errorf("first exchange round-tripped wrong: %+v", got[0])
	}
	if got[0].Ts.IsZero() {
		t.Error("timestamp was not stamped on append")
	}
	if len(got[0].Tools) != 2 {
		t.Errorf("tools = %v", got[0].Tools)
	}
}

func TestExchangeLogTruncatesLongResponses(t *testing.T) {
	log := NewExchangeLog(t.TempDir())
	if err := log.Append(Exchange{Prompt: "p", Response: strings.Repeat("x", maxStoredResponse+500)}); err != nil {
		t.Fatal(err)
	}
	got, _ := log.Load()
	if !strings.HasSuffix(got[0].Response, "…[truncated]") {
		t.Error("long response was not truncated")
	}
	if len(got[0].Response) > maxStoredResponse+20 {
		t.Errorf("truncated response still %d bytes", len(got[0].Response))
	}
}

func TestExchangeLogSkipsEmptyAndNil(t *testing.T) {
	var nilLog *ExchangeLog
	if err := nilLog.Append(Exchange{Prompt: "x"}); err != nil {
		t.Errorf("nil log should be a no-op, got %v", err)
	}
	log := NewExchangeLog(t.TempDir())
	if err := log.Append(Exchange{Prompt: ""}); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.Load(); len(got) != 0 {
		t.Errorf("empty prompt was logged: %+v", got)
	}
}

func TestExchangeLogMissingFileIsNotAnError(t *testing.T) {
	got, err := NewExchangeLog(t.TempDir()).Load()
	if err != nil || got != nil {
		t.Errorf("Load on missing file = (%v, %v), want (nil, nil)", got, err)
	}
}
