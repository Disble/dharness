package setup

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Disble/dharness/internal/preset"
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
	if entries, reason := WithdrawnLayerNote(p); reason != "" {
		notes = append(notes, report.Note{
			Kind:       "withdrawn",
			Actionable: true,
			Entries:    entries,
			Reason:     reason,
		})
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

// WithdrawnLayerNote names every ESLint layer dharness did not contribute
// because the project's own config already registers that plugin, and says
// what it costs.
//
// It exists so the withdrawal is never silent. Dropping a layer is the only
// way to keep a config that loads when two toolchains register one plugin
// key from two instances — flat config refuses that outright — but the two
// registrations are not equivalent, and saying nothing would let a project
// lose most of a rule set and read a clean `0 failed`. Measured on the
// project that reported this: dlinter-ts-react 0.9.0 pins react-doctor
// 0.7.4 with 395 rules while dharness installs 0.9.12 with 884, and 400 of
// the 621 rules dharness would have contributed are not enabled by the copy
// already there.
//
// It reports rather than resolves. Which build a project runs is the
// project's decision (§05), and neither answer is dharness's to pick: the
// resident copy keeps every `eslint-disable` comment and every CI grep
// working, and the newer one has the rules. What dharness owes is the fact
// that the choice is being made.
//
// It prints on every run, including a second identical sync, and that is a
// decision rather than a default. A note shown once is a note missed by
// whoever runs the second sync, and the thing it is guarding against is a
// rule set quietly shrinking — the exact failure that goes unnoticed for
// releases at a time. The cost is that a project syncing often reads it
// often; the alternative is that a project syncing often stops being told.
func WithdrawnLayerNote(p project.Project) (entries []string, reason string) {
	if !p.HasSource() {
		return nil, ""
	}

	all := preset.Layers(preset.Resolve(p))
	contributed := projectContributedLayers(p, all)

	kept := map[string]bool{}
	for _, layer := range contributed {
		kept[layerIdentity(layer)] = true
	}

	registered := registeredPlugins(p)
	packages := map[string]string{}
	registers := map[string]bool{}
	for _, layer := range all {
		if layer.Registers == "" || kept[layerIdentity(layer)] {
			continue
		}
		registers[layer.Registers] = true
		packages[layer.Registers] = layer.InstallName()
		entries = append(entries, layerIdentity(layer))
	}
	if len(entries) == 0 {
		return nil, ""
	}

	var named []string
	for plugin := range registers {
		named = append(named, fmt.Sprintf("%q", plugin))
	}
	sort.Strings(named)
	plugins := strings.Join(named, ", ")

	opening := versionGap(p, registers, packages, registered)
	if opening == "" {
		opening = fmt.Sprintf("this project's ESLint config already registers %s, from a package that is not dharness's. ", plugins)
	}

	return entries, opening + fmt.Sprintf(
		"It cannot be registered twice: flat config refuses one plugin key registered from two "+
			"instances, so contributing these layers as well would produce a config ESLint cannot "+
			"load at all. They are left out, and the copy already there is what runs. Two ways out, "+
			"and the choice is this project's: drop the package that brings the copy already here, "+
			"or keep it and accept the rules it does not carry. dharness contributes these layers "+
			"again by itself once nothing else registers %s.",
		plugins)
}

// versionGap opens the note with the fact a reader acts on, or with nothing
// when there is no fact to state.
//
// It leads rather than follows because it is the load-bearing sentence: the
// cost of a withdrawal is rules that silently stop being enforced, and a
// note that buries that behind the mechanism is a note that gets skimmed.
//
// Both numbers are already in hand at the moment this is written — the
// resident build comes out of the same `plugins` entry the withdrawal
// decision was made from, and the other is the package dharness installs,
// read from node_modules. The first version of this note described the gap
// in the abstract ("not necessarily the same build: if it is an older
// one…") while holding the concrete numbers, which asks the reader to first
// suspect a gap exists and then go measure it. Naming both turns a
// conditional into a fact, and a fact is what gets acted on.
//
// The conditional form survives only where a number is genuinely missing:
// an unreadable node_modules, or a plugin ESLint reported without a build
// string. Saying "0.7.4 against " with a blank would be worse than the
// hedge it replaced.
func versionGap(p project.Project, registers map[string]bool, packages, registered map[string]string) string {
	var gaps []string
	for plugin := range registers {
		resident := buildVersion(registered[plugin])
		ours := installedBuild(p, packages[plugin])
		if resident == "" || ours == "" || resident == ours {
			continue
		}
		gaps = append(gaps, fmt.Sprintf("%q here is %s, and dharness installs %s", plugin, resident, ours))
	}
	if len(gaps) == 0 {
		return ""
	}
	sort.Strings(gaps)

	return fmt.Sprintf(
		"the copy of %s — so every rule the newer build adds is not enforced on this project, "+
			"and nothing else will say so. ",
		strings.Join(gaps, "; "))
}

// buildVersion is the version out of ESLint's "name@version" plugin entry,
// or "" when the entry carries none.
//
// The name is dropped rather than printed: it is the plugin's own meta name,
// which for the case this note exists for is "react-doctor" — the same
// string as the key it is already shown under, and repeating it reads as two
// different things.
func buildVersion(entry string) string {
	_, version, found := strings.Cut(entry, "@")
	if !found {
		return ""
	}
	return version
}

// layerIdentity names one layer the way a person reading the note would:
// the specifier plus the property path the config sits at, which is what
// distinguishes react-doctor's three presets from each other.
//
// It doubles as this file's key for "is this the same layer", so the one
// thing it must be is total: every layer gets a name, an empty accessor
// included, and no branch decides which. A conditional here was mutation-
// tested into an equivalent, which is the shape of a branch that never
// needed to exist.
func layerIdentity(layer preset.Layer) string {
	return strings.TrimSpace(layer.Package + " " + strings.Join(layer.Accessor, "."))
}
