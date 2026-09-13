package staged

import "fmt"

// PartiallyStagedError reports a file dharness was about to scope for
// mutation that also carries changes beyond what was staged.
//
// Mutating exactly the staged ranges only answers the question this command
// exists to ask — would the commit under way survive mutation testing — when
// the staged ranges are the whole story for that file. A file with unstaged
// changes on top has more story than the index holds, and a scope built from
// the index alone would validate lines the commit will not actually contain.
type PartiallyStagedError struct{ Path string }

func (e *PartiallyStagedError) Error() string {
	return fmt.Sprintf(
		"%s is partially staged: it carries changes beyond what is indexed, so mutating only the staged ranges would validate lines this commit will not contain; stage the rest of the file or unstage it",
		e.Path,
	)
}

// checkFullyStaged fails closed the moment any file dharness is about to
// scope disagrees between the index and the working tree.
//
// Checked per file rather than once for the whole tree: a change can touch
// many files, and only the ones this run actually scoped for mutation matter
// here — a partially staged file dharness was never going to mutate is not
// this run's problem.
func checkFullyStaged(source string, paths []string) error {
	for _, path := range paths {
		out, err := gitOutput(source, "diff", "--name-only", "-z", "--", path)
		if err != nil {
			return fmt.Errorf("check %s for changes beyond the index: %w", path, err)
		}
		if len(out) > 0 {
			return &PartiallyStagedError{Path: path}
		}
	}
	return nil
}
