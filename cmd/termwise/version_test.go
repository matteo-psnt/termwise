package main

import (
	"runtime/debug"
	"testing"
)

func TestBuildVersion(t *testing.T) {
	stamped := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.4.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	tests := []struct {
		name           string
		version, sha   string
		info           *debug.BuildInfo
		wantV, wantSHA string
	}{
		{"go install @latest", "dev", "none", &debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}}, "v1.4.0", "none"},
		{"checkout build", "dev", "none", stamped, "v1.4.0", "0123456789ab-dirty"},
		{"devel", "dev", "none", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev", "none"},
		{"ldflags win", "1.2.3", "abc1234", stamped, "1.2.3", "abc1234"},
		{"no build info", "dev", "none", nil, "dev", "none"},
		{"pseudo-version", "dev", "none", withVersion(stamped, "v0.1.2-0.20261022043432-0123456789ab+dirty"), "v0.1.2-0.20261022043432-0123456789ab+dirty", "none"},
		{"pseudo-version, clean", "dev", "none", withVersion(stamped, "v0.0.0-20261022043432-0123456789ab"), "v0.0.0-20261022043432-0123456789ab", "none"},
		{"tag, dirty", "dev", "none", withVersion(stamped, "v1.4.0+dirty"), "v1.4.0+dirty", "none"},
	}
	for _, tt := range tests {
		v, c := buildVersion(tt.version, tt.sha, tt.info)
		if v != tt.wantV || c != tt.wantSHA {
			t.Errorf("%s: buildVersion = %q, %q; want %q, %q", tt.name, v, c, tt.wantV, tt.wantSHA)
		}
	}
}

func withVersion(info *debug.BuildInfo, v string) *debug.BuildInfo {
	out := *info
	out.Main.Version = v
	return &out
}
