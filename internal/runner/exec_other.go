//go:build !windows

package runner

// platformize is a no-op everywhere except Windows, where npm's .cmd shims
// cannot be executed directly.
func platformize(name string, args []string) (string, []string) {
	return name, args
}
