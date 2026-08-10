package project

import (
	"os"
	"path/filepath"
)

// Identity of a directory is the operating system's answer, never the string's.
//
// Two spellings name one directory for three reasons, and only one of them is
// recoverable from the text: a symlinked ancestor, a case-insensitive volume
// (the default on NTFS and APFS, so `Repo` and `repo` are one directory), and
// Unicode equivalence between the composed and decomposed spellings of an
// accented name. filepath.EvalSymlinks resolves the first and nothing else, so
// comparing strings after it still rejects two directories that are the same.
//
// os.Stat performs the lookup, which is the canonicalisation, and os.SameFile
// compares what the lookup returned. Neither operand is normalised first,
// because normalising one and not the other is how the comparison goes wrong.
//
// The mechanism is a lookup, so a path that does not exist has no identity.
// sameDirectory reports false for one, even against itself.
func sameDirectory(left, right string) bool {
	leftInfo, err := os.Stat(left)
	if err != nil || !leftInfo.IsDir() {
		return false
	}
	rightInfo, err := os.Stat(right)
	if err != nil || !rightInfo.IsDir() {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}

// contains reports whether path is root or lies beneath it.
//
// It climbs path's ancestors and stops at the first one that is the same
// directory as root, so a descendant that has not been created yet is still
// answered — by the deepest ancestor that does exist. The walk is bounded by
// the number of separators in path and ends at the volume root.
func contains(root, path string) bool {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return false
	}
	current, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for {
		if info, err := os.Stat(current); err == nil && info.IsDir() && os.SameFile(rootInfo, info) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}
