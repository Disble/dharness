package project

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// AmbiguousSourceError reports a repository with more than one place the
// package manager installs.
//
// This is the one question here that no derivation answers. Every other fact
// dharness needs is read off the tree; which of two independent JS projects a
// gate is meant to guard is intent, and intent is held by a person. Naming the
// candidates is the whole of what dharness can contribute.
type AmbiguousSourceError struct {
	Root       string
	Candidates []string
}

func (e *AmbiguousSourceError) Error() string {
	return fmt.Sprintf(
		"%s holds %d independent JS projects, and a gate guards one:\n\n    %s\n\nRun dharness from inside the one you mean.",
		e.Root, len(e.Candidates), strings.Join(e.Candidates, "\n    "),
	)
}

// lockfilePathspecs are what git is asked for.
//
// The lockfile is the signal rather than package.json, and the difference is
// not cosmetic: package.json appears at every level of a workspace, and a
// generated one appears in places nobody wrote — a Wails project carries
// frontend/wailsjs/runtime/package.json that no package manager installs from.
// A lockfile marks where the package manager actually installs, which is the
// directory dharness needs for node_modules/.bin and for the install command.
//
// It does not degrade in a monorepo: pnpm, bun and npm workspaces all keep one
// lockfile at the workspace root, which is also where node_modules lives.
var lockfilePathspecs = []string{
	"*bun.lockb",
	"*bun.lock",
	"*pnpm-lock.yaml",
	"*yarn.lock",
	"*package-lock.json",
}

// Discover resolves the two directories a repository has, which a conventional
// layout hides by making them the same one.
//
// Root is the repository: git, the hook manager, and the directory dharness
// owns. Source is where the package manager installs, which is what the wrapped
// tools have to run in. A Wails or monorepo layout separates them, and every
// question dharness asks belongs to exactly one of the two.
//
// Both are asked of git rather than found by walking. git already holds the
// answer to both, its index excludes node_modules for free, and it respects
// .gitignore — so there is no directory to skip, no depth to bound, and nothing
// to keep up to date. Outside a repository there is no split to make, and the
// given directory answers for both.
func Discover(dir string) (Project, error) {
	root, err := repositoryRoot(dir)
	if err != nil {
		// Not a repository. The commands that need one fail on their own, with
		// an error that says so; detection is not the place to raise it.
		return At(dir, dir), nil
	}

	candidates, err := sourceCandidates(root)
	if err != nil {
		return Project{}, err
	}

	switch len(candidates) {
	case 0:
		// No lockfile anywhere. Source stays empty rather than defaulting to
		// the root: reporting a JS project where there is none is what made a
		// Go repository look like an npm one.
		return inRepository(At(root, "")), nil
	case 1:
		return inRepository(At(root, candidates[0])), nil
	default:
		// A directory the caller is already standing in settles it without
		// asking, because standing there is an answer.
		for _, candidate := range candidates {
			if contains(candidate, dir) {
				return inRepository(At(root, candidate)), nil
			}
		}
		return Project{}, &AmbiguousSourceError{Root: root, Candidates: candidates}
	}
}

// inRepository marks a Project built from a resolved git root. It is not part
// of At itself: At is also used by Describe and by Discover's own swallow
// branch, neither of which found a repository.
func inRepository(p Project) Project {
	p.InRepository = true
	return p
}

// repositoryRoot asks git where the repository begins.
func repositoryRoot(dir string) (string, error) {
	out, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", &NotAGitRepositoryError{Dir: dir, Cause: err}
	}
	// git reports the toplevel with forward slashes on Windows, where every
	// path dharness joins onto it uses the separator the OS uses.
	return filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out)))), nil
}

// sourceCandidates lists every directory in the repository that a package
// manager installs into, nearest the root first.
func sourceCandidates(root string) ([]string, error) {
	args := append([]string{"ls-files", "-z", "--"}, lockfilePathspecs...)
	out, err := gitOutput(root, args...)
	if err != nil {
		return nil, &NotAGitRepositoryError{Dir: root, Cause: err}
	}

	var candidates []string
	for _, name := range splitNUL(out) {
		dir := filepath.Join(root, filepath.FromSlash(path.Dir(name)))
		if !known(candidates, dir) {
			candidates = append(candidates, dir)
		}
	}
	return candidates, nil
}

// known reports whether a directory is already among the candidates. It
// compares identity rather than spelling, because two lockfiles in one
// directory must not make it two candidates.
func known(candidates []string, dir string) bool {
	for _, candidate := range candidates {
		if sameDirectory(candidate, dir) {
			return true
		}
	}
	return false
}
