//go:build !windows

package staged

import "os"

// link creates destination as a symbolic link pointing at target.
//
// Outside Windows a plain symlink needs no elevated privilege and no
// separate command to shell out to.
func link(target, destination string) error {
	return os.Symlink(target, destination)
}
