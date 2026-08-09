package app

import (
	"runtime/debug"
	"strings"
)

// buildInfoReader is swappable for testing — prevents reading real BuildInfo in tests.
var buildInfoReader = debug.ReadBuildInfo

// ResolveVersion determines the effective version string.
// Priority: ldflags override > debug.BuildInfo.Main.Version > "dev".
//
// A release build passes a real version through ldflags and BuildInfo is never
// consulted. A `go install` build leaves the ldflags default at "dev", and
// BuildInfo carries the tagged version, or "(devel)" when untagged.
func ResolveVersion(ldflagsVersion string) string {
	if ldflagsVersion != "dev" {
		return ldflagsVersion
	}

	info, ok := buildInfoReader()
	if !ok {
		return "dev"
	}

	version := info.Main.Version
	if version == "" || version == "(devel)" {
		return "dev"
	}

	return strings.TrimPrefix(version, "v")
}
