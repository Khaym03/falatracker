package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestFillFromBuildInfo(t *testing.T) {
	bi := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.time", Value: "2026-11-06T10:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}

	var info Info
	fill(&info, bi)

	want := Info{Version: "v1.2.3", Commit: "abcdef1234567890", Date: "2026-11-06T10:00:00Z", Modified: true}
	if info != want {
		t.Errorf("fill() = %+v, want %+v", info, want)
	}
}

func TestLdflagsTakePrecedence(t *testing.T) {
	bi := &debug.BuildInfo{
		Main:     debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "fromvcs"}},
	}

	info := Info{Version: "v9.9.9", Commit: "fromldflags"}
	fill(&info, bi)

	if info.Version != "v9.9.9" || info.Commit != "fromldflags" {
		t.Errorf("ldflags values were overwritten: %+v", info)
	}
}

func TestDevelBuildIsIgnored(t *testing.T) {
	var info Info
	fill(&info, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}})
	if info.Version != "" {
		t.Errorf("Version = %q, want empty", info.Version)
	}
}

func TestGetAlwaysHasVersion(t *testing.T) {
	info := Get()
	if info.Version == "" || info.GoVersion == "" {
		t.Errorf("Get() = %+v, want Version and GoVersion set", info)
	}
}

func TestString(t *testing.T) {
	s := Info{Version: "v1.0.0", Commit: "abcdef1234567890", Date: "2026-11-06", Modified: true, GoVersion: "go1.27.0"}.String()
	want := "v1.0.0 (commit abcdef123456-dirty, built 2026-11-06, go1.27.0)"
	if s != want {
		t.Errorf("String() = %q, want %q", s, want)
	}

	if s := (Info{Version: "dev", GoVersion: "go1.27.0"}).String(); !strings.Contains(s, "commit unknown") {
		t.Errorf("String() = %q, want unknown commit", s)
	}
}
