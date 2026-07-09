// Package version derives a human-readable build identity for the lanfirst
// binaries. It needs no build flags: the git revision and dirty state come from
// the VCS stamp Go embeds on every `go build`, and the build time is read from
// the running executable's own modification time — so a plain rebuild always
// produces a visibly newer string, even with uncommitted changes.
//
// A release build may override the revision label by setting Tag via
// -ldflags "-X .../internal/version.Tag=v1.2.3".
package version

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Tag, when set at link time, replaces the git-derived revision label (used for
// tagged releases). Empty by default.
var Tag string

// ReleaseTag returns the semver tag this binary was released as (e.g. "v0.1.0"),
// or "" for dev/source builds. The updater uses this to decide whether the
// binary is on the release channel at all.
func ReleaseTag() string {
	return Tag
}

// revision returns the short git revision plus a "-dirty" suffix when the working
// tree had uncommitted changes at build time. Falls back to "unknown".
func revision() string {
	if Tag != "" {
		return Tag
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// buildTime returns when the running binary was written, formatted for display.
// It uses the executable's own modification time, which a rebuild always bumps.
func buildTime() string {
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return "unknown"
	}
	return fi.ModTime().Format("2006-01-02 15:04:05")
}

// String returns the full build identity, e.g. "abc1234-dirty (built 2026-06-24 15:10:33)".
func String() string {
	return fmt.Sprintf("%s (built %s)", revision(), buildTime())
}
