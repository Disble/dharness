//go:build windows

package runner

import (
	"path/filepath"
	"strings"
)

// platformize routes .cmd and .bat shims through cmd.exe.
//
// npm-installed binaries land in node_modules/.bin as .cmd shims on Windows,
// and CreateProcess cannot execute those directly. Every tool dharness wraps
// is installed that way, so without this the local path never works and every
// invocation silently falls back to the remote one.
func platformize(name string, args []string) (string, []string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cmd", ".bat":
		return "cmd", append([]string{"/c", name}, args...)
	default:
		return name, args
	}
}
