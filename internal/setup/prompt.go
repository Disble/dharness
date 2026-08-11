package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/tool"
)

// ArchitecturePrompt is architectureStep's Describe(p), the instruction body
// of the plan's one entry that is not a command.
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
//
// The fourth call site for preset.Resolve (design decision 4's other three
// are ownedFilesStep and boundariesOwnerStep): a matched preset may seed this
// prompt with structural facts its own framework documents — Next.js's
// router folders, for instance. A seed is offered as "confirm or correct
// this against the tree", never as a zone; §21 keeps zones with the agent,
// and an empty Seeds list (every non-framework project, and Wails and Expo
// today) renders nothing here at all, so this call site changes no output
// for a project no preset seeds.
func ArchitecturePrompt(p project.Project) string {
	owned := filepath.ToSlash(filepath.Join(project.Dir, ownedFallow))
	remote := tool.RemoteExec(p.PackageManager)
	fallow := tool.LatestSpec(tool.Fallow)

	var b strings.Builder
	b.WriteString(renderSeeds(preset.Seeds(preset.Resolve(p))))

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
	fmt.Fprintf(&b, "    %s %s list --boundaries\n\n", remote, fallow)
	b.WriteString("It prints every zone with the number of files it matched and the rules already\n")
	b.WriteString("expanded, and warns when a zone matched nothing — which is what a glob that\n")
	b.WriteString("does not fit this tree looks like. A zone with zero files is a mistake, not a\n")
	b.WriteString("style choice.\n\n")
	fmt.Fprintf(&b, "    %s %s dead-code --boundary-violations\n\n", remote, fallow)
	b.WriteString("Then read what the declaration costs today. Violations that already exist are\n")
	b.WriteString("reported but do not block: the gate fails only on the ones a change\n")
	b.WriteString("introduces, so an architecture can be declared on a codebase that does not\n")
	b.WriteString("yet obey it. Type-only imports do count as crossings, and imports of\n")
	b.WriteString("external packages do not.\n\n")

	b.WriteString("### The one rule dharness will not choose for you\n\n")
	b.WriteString("When a module splits into role files — `*.types.ts`, `*.helpers.ts` and the\n")
	b.WriteString("rest — does this project publish it through an `index.ts`, or does it import\n")
	b.WriteString("the concrete files?\n\n")
	b.WriteString("Both are architectures, not one right answer. A project can have good\n")
	b.WriteString("reasons to ban barrel files outright, and one that does cannot satisfy a rule\n")
	b.WriteString("demanding the file it deleted — every split module would report forever.\n\n")
	b.WriteString("So `folder-ownership` ships **off**. The other five rules are guardrails on\n")
	b.WriteString("generated code and stay on; this one is the only one that takes a side.\n\n")
	fmt.Fprintf(&b, "If this project does publish through barrels, turn it on in %s:\n\n", doctorPath(p))
	b.WriteString("    \"rules\": {\n")
	fmt.Fprintf(&b, "      %q: \"error\"\n", RulesPrefix+"/folder-ownership")
	b.WriteString("    }\n\n")
	b.WriteString("A severity written there survives every later run: dharness fills in only the\n")
	b.WriteString("rules a project has not answered for itself.\n")

	return b.String()
}

// renderSeeds renders the seed section, or "" when there is nothing to
// render — the byte-identity path a project no preset seeds depends on
// (every non-framework project, and Wails and Expo today, both of which
// contribute no seeds). Split out of ArchitecturePrompt so the empty/
// non-empty branch is testable against an exact seed count, not only
// against however many seeds a real preset happens to contribute.
func renderSeeds(seeds []preset.Seed) string {
	if len(seeds) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### What the framework already documents\n\n")
	for _, seed := range seeds {
		fmt.Fprintf(&b, "- %s\n  (%s)\n", seed.Text, seed.Because)
	}
	b.WriteString("\nConfirm or correct this against the tree. It names structure, not\n")
	b.WriteString("zones — those are still read from the code and the person who wrote it,\n")
	b.WriteString("never from this list.\n\n")
	return b.String()
}

// doctorPath names react-doctor's config as the reader sees it from the
// repository root, which is where the plan is read but not always where the
// JS project lives.
func doctorPath(p project.Project) string {
	if rel := p.SourceRel(); rel != "" {
		return rel + "/" + doctorConfig
	}
	return doctorConfig
}
