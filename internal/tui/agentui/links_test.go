package agentui

import "testing"

// Punctuation ending the sentence is not part of the URL, so neither the
// opened URL nor its clickable span includes it.
func TestFindLinksTrimsTrailingPunctuation(t *testing.T) {
	for content, want := range map[string]string{
		"see https://example.com/docs.":    "https://example.com/docs",
		"at https://example.com/a?b=1, ok": "https://example.com/a?b=1",
		"https://example.com/x.html":       "https://example.com/x.html",
	} {
		links := findLinks(content)
		if len(links) != 1 {
			t.Fatalf("%q: got %d links", content, len(links))
		}
		l := links[0]
		if l.URL != want {
			t.Errorf("%q: URL = %q, want %q", content, l.URL, want)
		}
		if l.EndX-l.StartX != len(want) {
			t.Errorf("%q: span %d..%d, want %d columns", content, l.StartX, l.EndX, len(want))
		}
	}
}
