package setup

import (
	"fmt"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// This file owns the safe side of the S2/S3 split: when the analysis finds
// no conflict, the reference is added without touching a byte the project
// wrote; when it finds one — or the file cannot take an additive reference —
// the agent gets the precise instruction instead of a rewritten file.
//
// dharness never deletes a consumer command and never rewrites a package
// script. Both are the project's decisions; sync either adds one reference
// key or explains which edit the agent owns.

// lefthookExtendsSupport reports whether raw can take the additive reference
// wireLefthookExtends writes, plus the reason when it cannot: which
// unsupported form the file actually has, so the reconciliation names the
// real shape instead of always blaming an `extends` key the file may not have.
//
// Supported is a plain block mapping with no top-level `extends` key: a
// second entry would duplicate the key and damage the mapping, and anything
// that is not a block mapping — an explicit `---` document, directives, a
// flow or sequence shape, tab indentation — cannot take a prepended block
// without changing what the document means. An unsupported shape is precise
// reconciliation, never a rewritten file.
func lefthookExtendsSupport(raw []byte) (bool, string) {
	if strings.Contains(string(raw), "\t") {
		return false, "uses tab indentation, which YAML forbids, so no key can be added safely"
	}
	first := true
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(trimmed, "---") {
				return false, "starts with an explicit `---` document marker, so a prepended block would open a second document"
			}
			if strings.HasPrefix(trimmed, "%") {
				return false, "opens with YAML directives, which a prepended block would separate from the document they govern"
			}
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
				return false, "is a flow or sequence document rather than a block mapping, so a prepended mapping key would not land in the same document"
			}
			if name, _, ok := strings.Cut(trimmed, ":"); !ok || strings.TrimSpace(name) == "" {
				return false, "does not open with a block mapping key, so a prepended mapping key would not land in the same document"
			}
		}
		// A top-level key opens at column zero. The tab refusal above already
		// answered a tab-indented file, and the trimmed-blank continue dropped
		// every empty and all-whitespace line, so the only indentation left to
		// exclude here is a leading space.
		if line[0] != ' ' {
			if name, _, ok := strings.Cut(trimmed, ":"); ok && strings.TrimSpace(name) == "extends" {
				return false, "already carries a top-level `extends` key, which a second entry would duplicate"
			}
		}
	}
	return true, ""
}

// unsupportedLefthookShapeReason tells the agent which edit the file needs:
// the reference, added by hand, preserving everything the file declares. It
// names the actual unsupported form, because the fix for a document marker
// differs from the fix for a competing `extends` key, and renders the exact
// lines, because the fix is a merge and the agent is the one doing it.
func unsupportedLefthookShapeReason(p project.Project, form string) string {
	if form == "" {
		form = "a shape the automatic wiring cannot extend"
	}
	return fmt.Sprintf(
		"%s carries %s. dharness will not rewrite it: the additive reference only lands\nin a plain block mapping. Add the managed reference yourself, preserving\neverything the file already declares:\n\n    extends:\n      - %s\n\nThere is exactly one complete Vitest testing gate: the managed coverage-tier\ninvocation the referenced file runs. Keep unrelated checks as they are; if the\nexisting entries run the full Vitest suite, resolve that duplicate first and\nleave the managed gate as the one.",
		lefthookConfig, form, ownedFrom(p, p.Root, ownedLefthook))
}

// vitestGateReconciliation is the targeted S3/S4 instruction: the concrete
// file and command, the single complete Vitest testing gate including the
// coverage tiers, and the autonomous resolution the consuming agent owns.
// A confirmed duplicate says so; an uncertainty says it is uncertain —
// unknown is not confirmed Vitest, and the diagnostics reflect that.
func vitestGateReconciliation(p project.Project, finding vitestGateFinding) string {
	entry, err := coverageTiersEntryFromSource(p)
	managed := "vitest run --coverage --config " + coverageBridgeName
	if err == nil {
		managed = "vitest run --coverage --config " + entry
	}

	var b strings.Builder
	if finding.through != "" {
		fmt.Fprintf(&b, "%s runs `%s` (via package script %s)",
			finding.where, finding.command, finding.through)
	} else {
		fmt.Fprintf(&b, "%s runs `%s`", finding.where, finding.command)
	}

	if finding.status == vitestGateEquivalent {
		b.WriteString(", a complete Vitest suite — the same suite the managed gate runs with\nthe coverage tiers enforced (" + managed + ").\n")
	} else {
		reason := finding.reason
		if reason == "" {
			reason = "it could not be resolved to a concrete invocation"
		}
		b.WriteString(", which could not be resolved to a concrete invocation: " + reason + ".\n" +
			"Unknown is not confirmed Vitest, so this is reported as uncertainty rather than\nassumed safe.\n")
	}

	b.WriteString("\nThere is exactly one complete Vitest testing gate: the managed coverage-tier\ninvocation. Running a second full suite beside it executes the same tests twice on\nevery commit, so resolve the conflict yourself: ")
	if finding.status == vitestGateEquivalent {
		b.WriteString("keep the managed gate and remove the duplicate full-suite invocation from\n")
		if finding.through != "" {
			b.WriteString("the package script it reaches")
		} else {
			fmt.Fprintf(&b, "%s", finding.where)
		}
		b.WriteString(", leaving unrelated checks, focused or watch runs, and alternate configs\nuntouched. ")
	} else {
		b.WriteString("if it runs the full suite, remove it and keep the managed gate; if it is\nscoped, an alternate config, or not Vitest at all, narrow it so the analysis can\nsee that. ")
	}
	b.WriteString("dharness never deletes consumer commands or rewrites package scripts; until\none complete gate remains, this step stays with you.")
	return b.String()
}
