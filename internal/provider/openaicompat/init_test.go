package openaicompat

import "testing"

func TestWithAPIPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"http://localhost:11434", "http://localhost:11434/v1"},
		{"http://localhost:11434/", "http://localhost:11434/v1"},
		{"http://192.168.1.5:11434", "http://192.168.1.5:11434/v1"},
		{"http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"https://proxy.example.com/ollama/v1", "https://proxy.example.com/ollama/v1"},
		{"not a url", "not a url"},
	}
	for _, tt := range tests {
		if got := withAPIPath(tt.in, "/v1"); got != tt.want {
			t.Errorf("withAPIPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
