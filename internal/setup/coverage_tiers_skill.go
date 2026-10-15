package setup

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/tiers"
)

// coverageTiersSkillAsset is the one managed source of the coverage-tiers
// guidance. `go:embed` keeps it in the binary rather than rebuilding it from a
// Go string literal, so what this repository reads and what a consumer
// receives are the same bytes by construction — the pattern
// coverage_tiers_runtime.go already established for the runtime module.
//
// Nothing here is a second classification authority: the skill explains the
// procedure, the manifest declares the classification, and the resolver behind
// `dharness policy` answers it. This step owns bytes, never a tier.
//
//go:embed assets/coverage-tiers/SKILL.md
var coverageTiersSkillAsset []byte

// coverageTiersSkill returns the guidance's bytes.
//
// The returned slice is a copy, for the same reason coverageTiersModule's is:
// a caller that trims what it got would otherwise corrupt the embedded asset
// for the rest of the process, and the failure would surface later as an
// unrelated consumer writing a truncated skill.
func coverageTiersSkill() []byte {
	return bytes.Clone(coverageTiersSkillAsset)
}

// The marker pair this step owns inside the instruction files the project
// already keeps.
//
// They are located with locateCoverageTiersRegion — the same line-bounded,
// text-agnostic grammar coverage_tiers_config.go uses for the consumer's Vitest
// config — rather than a second region grammar: a begin with no end, an end
// before its begin, or more than one of either is malformed, and a malformed
// region is refused instead of repaired by guesswork.
const (
	coverageTiersGuideBegin = "<!-- dharness:coverage-tiers begin — rewritten by `dharness sync`; edits here are lost. -->"
	coverageTiersGuideEnd   = "<!-- dharness:coverage-tiers end -->"
)

// coverageTiersGuideDoc is one instruction file the project already keeps, and
// the skill path that file's readers load.
//
// The two are paired because they are not interchangeable: AGENTS.md is a
// convention several clients read, and the shared location is
// .agents/skills/; CLAUDE.md is Claude's own file, and Claude loads its skill
// from .claude/skills/. A single reference naming the wrong one sends the
// reader to a path their client never checks.
type coverageTiersGuideDoc struct {
	name  string
	skill string
}

var coverageTiersGuideDocs = []coverageTiersGuideDoc{
	{name: agentsInstructions, skill: ".agents/skills/coverage-tiers/SKILL.md"},
	{name: claudeInstructions, skill: ".claude/skills/coverage-tiers/SKILL.md"},
}

// coverageTiersSkillDirs are the two copies this step owns, one per client
// named in the design. They are copies rather than links on purpose: a symlink
// needs a Windows privilege the consuming checkout does not have, and the
// asset is small enough that one more copy costs less than the failure mode.
var coverageTiersSkillDirs = []string{
	filepath.Join(".claude", "skills", "coverage-tiers"),
	filepath.Join(".agents", "skills", "coverage-tiers"),
}

// coverageTiersSkillFile is one file this step owns, with the exact bytes it
// writes.
type coverageTiersSkillFile struct {
	path string
	data []byte
}

// coverageTiersSkillPlan is everything the step derives, computed once per call
// so Satisfied, Describe, Delegated and Apply cannot disagree about it (§07).
//
// conflict and files are mutually exclusive: a refusal is the whole answer, and
// a plan that says "dharness will not write this" never also says "here are the
// bytes" (§20 — a blocked step descends a rung, it does not half-write).
type coverageTiersSkillPlan struct {
	conflict string
	files    []coverageTiersSkillFile
}

// satisfies reports whether every byte this plan would write is already on
// disk, which is the whole of this step's idempotence: the canonical bytes are
// read back, never remembered.
func (plan coverageTiersSkillPlan) satisfies() bool {
	for _, file := range plan.files {
		existing, err := os.ReadFile(file.path)
		if err != nil || !bytes.Equal(existing, file.data) {
			return false
		}
	}
	return true
}

// coverageTiersSkillStep distributes the guidance to a repository that
// declared the tiers.
//
// The declaration is the whole switch, exactly as it is for coverageTiersStep:
// a repository with no coverage-tiers.json and no managed copy of this step's
// own gets nothing, and dharness never creates an instruction file to hold a
// reference. The step follows the declaration rather than the wiring, so a
// declared repository whose native integration is still incomplete gets the
// procedure that explains what is missing.
type coverageTiersSkillStep struct{}

func (coverageTiersSkillStep) ID() string {
	return "distribute the coverage-tiers skill and its agent references"
}

func (coverageTiersSkillStep) Describe(project.Project) string {
	return "The tiers are only useful to whoever is editing the adopted source, so the guidance\n" +
		"that explains them ships with this step: the same skill body at\n" +
		".claude/skills/coverage-tiers/SKILL.md and .agents/skills/coverage-tiers/SKILL.md, plus\n" +
		"a managed block in AGENTS.md and CLAUDE.md where those files already exist. A\n" +
		"repository that declares no tiers gets none of it, and neither instruction file is\n" +
		"ever created by dharness."
}

// Satisfied is a byte comparison over everything this step owns, and a refusal
// is never satisfied: a destination dharness cannot act on is reported by
// Delegated, and run() asks Satisfied first, so answering true there would file
// a refusal under "Already in place".
func (coverageTiersSkillStep) Satisfied(p project.Project) bool {
	plan := planCoverageTiersSkill(p)
	return plan.conflict == "" && plan.satisfies()
}

// Delegated hands over every state dharness will not write: a destination that
// is not a regular file, an instruction file whose managed region cannot be
// bounded, and the managed copies a removed declaration leaves behind. Each
// reason is the fix, because none of them is a state this step may invent its
// way out of (§05, §20).
func (coverageTiersSkillStep) Delegated(p project.Project) (string, bool) {
	plan := planCoverageTiersSkill(p)
	if plan.conflict == "" {
		return "", false
	}
	return plan.conflict, true
}

// Apply writes the copies and references this repository's declaration needs.
// It is unreachable while plan.conflict is set — Delegated answers first — and
// the error is the contract assertion every other unreachable Apply in this
// package keeps.
//
// Every write goes through the run's Writer, so a later failure puts back the
// copies and the instruction files this run already wrote.
func (coverageTiersSkillStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	plan := planCoverageTiersSkill(p)
	if plan.conflict != "" {
		return Facts{}, errors.New(plan.conflict)
	}

	for _, file := range plan.files {
		if existing, err := os.ReadFile(file.path); err == nil && bytes.Equal(existing, file.data) {
			continue
		}
		if err := w.Write(file.path, file.data); err != nil {
			return Facts{}, err
		}
	}
	return Facts{}, nil
}

// planCoverageTiersSkill derives this repository's guidance wiring, or the
// reason there is none.
//
// The order is the cheapest answer first: the declaration (one stat), the
// managed copies (one Lstat each) and only then the instruction files, which
// are read and located. Nothing here writes, starts a process or installs
// anything.
func planCoverageTiersSkill(p project.Project) coverageTiersSkillPlan {
	if !coverageTiersDeclared(p) {
		if reason := coverageTiersSkillLeftBehind(p); reason != "" {
			return coverageTiersSkillPlan{conflict: reason}
		}
		return coverageTiersSkillPlan{}
	}

	files := make([]coverageTiersSkillFile, 0, len(coverageTiersSkillDirs)+len(coverageTiersGuideDocs))
	for _, dir := range coverageTiersSkillDirs {
		file, reason := coverageTiersSkillCopy(p, dir)
		if reason != "" {
			return coverageTiersSkillPlan{conflict: reason}
		}
		files = append(files, file)
	}
	for _, doc := range coverageTiersGuideDocs {
		file, reason := coverageTiersGuideReference(p, doc)
		if reason != "" {
			return coverageTiersSkillPlan{conflict: reason}
		}
		// nil means the project does not keep that instruction file: the
		// reference is only ever added where a file already exists, and
		// dharness never creates one.
		if file != nil {
			files = append(files, *file)
		}
	}
	return coverageTiersSkillPlan{files: files}
}

// coverageTiersSkillCopy is one client's copy path, or the reason it cannot be
// written.
//
// The destination is checked with Lstat rather than Stat because a symlink is
// the case that matters: os.WriteFile follows one, so writing through it would
// modify a file this step does not own, somewhere else. A destination that
// exists and is not a plain file is refused with the fix instead of being
// replaced or followed. Anything else — including a path this process cannot
// inspect at all — falls through to the write, whose own error names the file
// and rolls the run back rather than half-writing it.
func coverageTiersSkillCopy(p project.Project, dir string) (coverageTiersSkillFile, string) {
	path := filepath.Join(p.Root, dir, "SKILL.md")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return coverageTiersSkillFile{}, fmt.Sprintf(
			"%s exists and is not a regular file, so writing the coverage-tiers skill there would\nfollow a link or replace a directory this step does not own.\n\nRemove it (or make it a regular file), then run `dharness sync` again; until then\nnothing is written.",
			rootRelative(p, path))
	}
	return coverageTiersSkillFile{path: path, data: coverageTiersSkill()}, ""
}

// coverageTiersGuideReference is the project's instruction file with the
// managed region inserted or replaced, nil when the project does not keep that
// file, or the reason it cannot be touched.
func coverageTiersGuideReference(p project.Project, doc coverageTiersGuideDoc) (*coverageTiersSkillFile, string) {
	path := filepath.Join(p.Root, doc.name)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, fmt.Sprintf(
			"%s exists but could not be read: %v\n\nThe coverage-tiers reference belongs in that file, and this step will not rewrite\none it cannot read. Fix it (or remove it), then run `dharness sync` again; until\nthen nothing is written.",
			doc.name, err)
	}

	body := string(raw)
	ending := regionLineEnding(body, 0)
	region := coverageTiersGuideRegion(doc, ending)

	switch located := locateCoverageTiersRegion(body, coverageTiersGuideBegin, coverageTiersGuideEnd); located.state {
	case markersMalformed:
		return nil, fmt.Sprintf(
			"%s carries a partial or duplicated managed coverage-tiers region: %d begin marker(s) and\n%d end marker(s).\n\nDelete every line carrying one of these two markers, or restore the missing half, then\nrun `dharness sync` again — dharness will not rewrite a region it cannot bound:\n\n    %s\n    %s",
			doc.name, strings.Count(body, coverageTiersGuideBegin), strings.Count(body, coverageTiersGuideEnd),
			coverageTiersGuideBegin, coverageTiersGuideEnd)
	case markersPresent:
		return &coverageTiersSkillFile{path: path, data: replaceRange(raw, located.from, located.to, region)}, ""
	default:
		return &coverageTiersSkillFile{path: path, data: appendCoverageTiersGuideRegion(raw, region, ending)}, ""
	}
}

// coverageTiersGuideRegion renders the managed region for one instruction file:
// the tiers, the declaration, and the one-line pointer at the skill and the
// guide.
//
// The region is rendered with the file's own line ending rather than always
// "\n", so a CRLF repository does not gain a mixed-ending block, and the bytes
// on the next run still round-trip.
func coverageTiersGuideRegion(doc coverageTiersGuideDoc, ending string) string {
	return strings.Join([]string{
		coverageTiersGuideBegin,
		"Coverage targets: CORE 100%, IMPORTANT 80%, INFRASTRUCTURE 0%; classification lives",
		"in `coverage-tiers.json`, and the wired gate enforces it. Before changing one, read",
		"`" + doc.skill + "`, shipped by the same binary that wired this gate. dharness owns",
		"the guide that explains the tiers; this region names no path a project lacks.",
		coverageTiersGuideEnd,
		"",
	}, ending)
}

// appendCoverageTiersGuideRegion adds the region at the end of the project's
// own file, on a fresh line and separated by one blank line. Appending is the
// only insertion this step performs: it cannot land between a heading and the
// paragraph that explains it, and it never rewrites a byte the project wrote.
func appendCoverageTiersGuideRegion(raw []byte, region, ending string) []byte {
	body := string(raw)
	if body != "" {
		if !strings.HasSuffix(body, ending) {
			body += ending
		}
		if !strings.HasSuffix(body, ending+ending) {
			body += ending
		}
	}
	return []byte(body + region)
}

// coverageTiersSkillLeftBehind names the bytes this step owns that survive a
// removed declaration, or "".
//
// It is a real conflict and not a cleanup, for the same reason
// coverageTiersLeftBehind is: whether the declaration is coming back is the
// project's decision, and a guidance step that deleted its own copies on a
// missing manifest would take the explanation away from whoever is looking for
// the reason the gate stopped enforcing (§05, §20).
func coverageTiersSkillLeftBehind(p project.Project) string {
	found := coverageTiersSkillFootprint(p)
	if len(found) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"there is no %s, but the managed coverage-tiers guidance it activated is still here:\n%s\n\nRestore %s, or remove the managed copy/copies and the marked region(s) to drop the\nadoption.",
		tiers.ManifestName, "  "+strings.Join(found, "\n  "), tiers.ManifestName)
}

// coverageTiersSkillFootprint names the managed bytes of this step that are
// present, relative to the repository root and in a stable order, or nil when
// there are none.
//
// Every one of them is read, never remembered (§07): the copies are files
// dharness owns, and the managed region is the reference it wrote into a file
// the project owns. A repository with none of them and no declaration is a
// repository this step has never touched.
func coverageTiersSkillFootprint(p project.Project) []string {
	if p.Root == "" {
		return nil
	}

	var found []string
	for _, dir := range coverageTiersSkillDirs {
		rel := filepath.ToSlash(filepath.Join(dir, "SKILL.md"))
		if _, err := os.Lstat(filepath.Join(p.Root, dir, "SKILL.md")); err == nil {
			found = append(found, rel)
		}
	}
	for _, doc := range coverageTiersGuideDocs {
		raw, err := os.ReadFile(filepath.Join(p.Root, doc.name))
		if err != nil {
			continue
		}
		// A half-written region is a leftover too: it is a marker this step
		// wrote or was meant to bound, and reporting it is how its owner finds
		// it after the declaration is gone.
		if locateCoverageTiersRegion(string(raw), coverageTiersGuideBegin, coverageTiersGuideEnd).state != markersAbsent {
			found = append(found, doc.name)
		}
	}
	return found
}
