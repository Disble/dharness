//go:build windows

package staged

import (
	"bytes"
	"fmt"

	"github.com/Disble/dharness/internal/runner"
)

// link creates destination as a directory junction pointing at target.
//
// A junction rather than os.Symlink: a Windows symbolic link needs an
// elevated process or Developer Mode to create, and this runs inside a
// pre-commit hook on whatever privilege level the developer's shell already
// has. `mklink /J` needs neither.
func link(target, destination string) error {
	var stderr bytes.Buffer
	cmd := runner.Command{
		Label: "mklink",
		Name:  "cmd",
		Args:  []string{"/c", "mklink", "/J", destination, target},
	}
	if err := runner.Run(cmd, &bytes.Buffer{}, &stderr); err != nil {
		return fmt.Errorf("link %s to %s: %w: %s", destination, target, err, stderr.String())
	}
	return nil
}
