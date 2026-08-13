package setup

import (
	"path/filepath"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
)

// collectNotes reshapes the three existing prose functions into
// []report.Note — one unified home for what Run reads before the first
// byte changes (design.md Decision 8). Their order matches what RunSync
// printed before this slice: a config dharness could not check, then what
// a matched preset had to assume, then residue a retired mechanism left
// behind.
func collectNotes(p project.Project) []report.Note {
	var notes []report.Note

	if reason := UncheckableConfigNote(p); reason != "" {
		notes = append(notes, report.Note{Kind: "not-checked", Reason: reason})
	}
	if reason := UncertainPresetNote(p); reason != "" {
		notes = append(notes, report.Note{Kind: "assumed", Reason: reason})
	}
	if entries, path, reason := eslintResidue(p); len(entries) > 0 {
		notes = append(notes, report.Note{
			Kind:    "residue",
			Path:    filepath.ToSlash(path),
			Entries: entries,
			Reason:  reason,
		})
	}

	return notes
}
