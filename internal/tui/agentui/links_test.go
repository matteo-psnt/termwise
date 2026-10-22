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

// glamour can wrap links in OSC 8 hyperlinks, terminated by BEL or ST. Their
// bytes are not visible, so neither the URL nor its columns may include them.
func TestFindLinksSkipsOSCHyperlinks(t *testing.T) {
	for _, content := range []string{
		"see \x1b]8;;https://a.dev\x07https://a.dev\x1b]8;;\x07 now",
		"see \x1b]8;;https://a.dev\x1b\\https://a.dev\x1b]8;;\x1b\\ now",
		"see \x1b]8;;https://a.dev\x1b\\\x1b[4mhttps://a.dev\x1b[0m\x1b]8;;\x1b\\ now",
	} {
		links := findLinks(content)
		if len(links) != 1 {
			t.Fatalf("%q: got %d links (%+v), want 1", content, len(links), links)
		}
		if l := links[0]; l.URL != "https://a.dev" || l.StartX != 4 || l.EndX != 17 {
			t.Errorf("%q: got %+v, want https://a.dev at 4..17", content, l)
		}
	}
}
