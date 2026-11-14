// Package version reports which build of falatracker is running.
//
// Since Go 1.24, `go build` embeds the module version (from the git tag) and
// the VCS revision automatically, so a plain build already has version info.
// The variables below can still be overridden at build time:
//
//	go build -ldflags "-X github.com/Khaym03/falatracker/internal/version.Version=v1.2.0"
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Build-time overrides. Empty values fall back to the embedded build info.
var (
	Version = ""
	Commit  = ""
	Date    = ""
)

// Info describes the running build.
type Info struct {
	Version   string
	Commit    string
	Date      string
	Modified  bool // built from a git tree with uncommitted changes
	GoVersion string
}

// Get returns the build information.
func Get() Info {
	info := Info{Version: Version, Commit: Commit, Date: Date, GoVersion: runtime.Version()}
	if bi, ok := debug.ReadBuildInfo(); ok {
		fill(&info, bi)
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}

// fill completes the empty fields of info from bi.
func fill(info *Info, bi *debug.BuildInfo) {
	if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
}

// String formats the info on one line, e.g.
// "v1.2.0 (commit 1a2b3c4d5e6f, built 2026-11-06T10:00:00Z, go1.27.0)".
func (i Info) String() string {
	commit := i.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if commit == "" {
		commit = "unknown"
	}
	if i.Modified {
		commit += "-dirty"
	}

	date := i.Date
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("%s (commit %s, built %s, %s)", i.Version, commit, date, i.GoVersion)
}
