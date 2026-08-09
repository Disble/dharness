package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// ArchitecturePrompt is the last thing init prints, and the only part of
// adoption that is not a command.
//
// It is a prompt about how to do an analysis, not a question with options.
// Asking "which preset?" would already presuppose the answer: zones encode
// what the code is meant to be, and that is read from the code and from the
// person who wrote it, not detected. fallow agrees — its own `recommend`
// classifies decisions into detected, defaulted and taste, and declines to
// propose boundaries at all.
//
// It names the commands the analysis needs, because they exist, and it names
// the one file to write, because pointing the work at a single file is what
// keeps the project's own configuration untouched.
func ArchitecturePrompt(p project.Project) string {
	owned := filepath.ToSlash(filepath.Join(project.Dir, ownedFallow))
	remote := p.RemoteExec()

	var b strings.Builder
	b.WriteString("## Left to you: decide this project's architecture\n\n")
	b.WriteString("Everything above was a command. This is not: architecture boundaries say what\n")
	b.WriteString("the code is meant to be, and no tool can read intent off a tree. Do the\n")
	b.WriteString("analysis below and write the result — nothing else needs to change.\n\n")

	b.WriteString("### What to find out\n\n")
	b.WriteString("1. Read the source layout. What are the real seams: features, layers, a\n")
	b.WriteString("   delivery shell around a domain, a monorepo of packages that repeat the same\n")
	b.WriteString("   internal shape?\n")
	b.WriteString("2. Follow the imports that cross those seams today. A boundary that half the\n")
	b.WriteString("   codebase already violates is a wish, not an architecture — say so rather\n")
	b.WriteString("   than declaring it and drowning the gate.\n")
	b.WriteString("3. Decide whether one of the presets already describes this project:\n")
	b.WriteString("   `bulletproof`, `layered`, `hexagonal` or `feature-sliced`. A preset is one\n")
	b.WriteString("   line and expands into zones and rules on its own.\n")
	b.WriteString("4. If none fits, declare zones by glob. `autoDiscover` isolates every feature\n")
	b.WriteString("   under a directory without listing them; `root` scopes a zone to one package\n")
	b.WriteString("   of a monorepo, which is how several packages share an internal layout\n")
	b.WriteString("   without sharing zones.\n")
	b.WriteString("5. Ask the person you are working with whatever the code cannot answer. This\n")
	b.WriteString("   is a decision about intent, and they hold it.\n\n")

	b.WriteString("### Where to write it\n\n")
	fmt.Fprintf(&b, "    %s\n\n", owned)
	b.WriteString("That file is already referenced from this project's fallow configuration, so\n")
	b.WriteString("writing the `boundaries` block there is the whole change.\n\n")

	b.WriteString("### How to check the result\n\n")
	fmt.Fprintf(&b, "    %s fallow list --boundaries\n\n", remote)
	b.WriteString("It prints every zone with the number of files it matched and the rules already\n")
	b.WriteString("expanded, and warns when a zone matched nothing — which is what a glob that\n")
	b.WriteString("does not fit this tree looks like. A zone with zero files is a mistake, not a\n")
	b.WriteString("style choice.\n\n")
	fmt.Fprintf(&b, "    %s fallow dead-code --boundary-violations\n\n", remote)
	b.WriteString("Then read what the declaration costs today. Violations that already exist are\n")
	b.WriteString("reported but do not block: the gate fails only on the ones a change\n")
	b.WriteString("introduces, so an architecture can be declared on a codebase that does not\n")
	b.WriteString("yet obey it. Type-only imports do count as crossings, and imports of\n")
	b.WriteString("external packages do not.\n")

	return b.String()
}
