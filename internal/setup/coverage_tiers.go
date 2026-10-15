package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/tiers"
	"github.com/Disble/dharness/internal/tool"
)

// coverageTiersStep is the one opt-in step of the plan: the coverage policy
// exists only where a project declares it.
//
// The consumer's own coverage-tiers.json is the whole switch (§07 — state is
// read from the repository, never recorded). Without it this step offers
// adoption to the agent but writes nothing: no managed module, bridge entry,
// splice, gate line or runner invocation. That is why it is last in Plan(): an
// adopted repository's gate line is added beside the gate line hookInstallStep
// has already written — the check inside .dharness/lefthook.yml for lefthook,
// the appended line in .husky/pre-commit for husky — and neither exists before
// that step runs.
//
// Everything this step writes is derived on the call. The tiers themselves are
// never read here: the Go resolver behind `dharness policy` answers the runtime
// at load time, and this step owns only the bytes that carry the question to
// it. Classification, merging and the numbers are the resolver's and the
// consumer's, never this file's (§17).
type coverageTiersStep struct{}

const (
	// coverageBridgeName is the entry the native gate run is pointed at with
	// --config. It is the file that makes an incomplete adoption fail at load
	// instead of measuring nothing: it re-exports the project's own config and
	// refuses to load when that config never gained the managed regions.
	coverageBridgeName = "vitest.coverage.mjs"

	// coverageHuskyMarker identifies the single line this step owns inside a
	// husky hook. husky keeps a shell script that is the project's own file,
	// so the line is marked and replaced in place rather than re-appended —
	// the same reason owned.go's regions exist, for a file dharness does not
	// own outright.
	coverageHuskyMarker = "# dharness:coverage"
)

// coverageVitestConfigNames are the names Vitest resolves before it falls back
// to the Vite config, and coverageViteConfigNames is that fallback, both in
// Vite's own documented order.
//
// Detection is a filesystem question and stops there: which config a project
// actually uses is answered by the runner reading it, and dharness never
// evaluates JavaScript to find out (§01, §09). A project carrying two of these
// is answered by the first in this order — a stated boundary rather than a
// guess, and one the bridge entry makes harmless: whatever dharness wired is
// what the gate points Vitest at, so the run and the wiring cannot disagree.
var (
	coverageVitestConfigNames = []string{"vitest.config.js", "vitest.config.mjs", "vitest.config.ts", "vitest.config.cjs", "vitest.config.mts", "vitest.config.cts"}
	coverageViteConfigNames   = []string{"vite.config.js", "vite.config.mjs", "vite.config.ts", "vite.config.cjs", "vite.config.mts", "vite.config.cts"}
)

func (coverageTiersStep) ID() string { return "enforce the coverage tiers in the native test run" }

func (coverageTiersStep) Describe(project.Project) string {
	return fmt.Sprintf(
		"coverage-tiers.json declares this repository's coverage tiers, and the gate runs the\nproject's own Vitest with coverage so that they are enforced. The tiers arrive at\n%s, and the project's own config is pointed at the bridge entry\nbeside it. A repository that declares no tiers gets none of this.",
		filepath.ToSlash(filepath.Join(project.Dir, coverageTiersRuntimeName)))
}

// Satisfied is a byte comparison over everything this step owns, and a refusal
// is never satisfied: a declaration dharness cannot act on is reported by
// Delegated, and run() asks Satisfied first, so answering true there would file
// a refusal under "Already in place".
func (coverageTiersStep) Satisfied(p project.Project) bool {
	plan := planCoverageTiers(p)
	return plan.conflict == "" && plan.satisfies()
}

// Delegated hands over every state dharness will not write: a declaration the
// resolver refuses, a project with no Vitest config to point at, a runner that
// is not installed, a config the splice would have to guess at. Each reason is
// the fix, because none of them is a step dharness may take on the project's
// behalf (§05, §18).
func (coverageTiersStep) Delegated(p project.Project) (string, bool) {
	plan := planCoverageTiers(p)
	if plan.conflict == "" {
		return "", false
	}
	return plan.conflict, true
}

// Apply writes the managed bytes this repository's declaration needs. It is
// unreachable while plan.conflict is set — Delegated answers first — and the
// error is the contract assertion every other unreachable Apply in this
// package keeps.
//
// Every write goes through the run's Writer, so a later failure in this same
// step puts back the module, the bridge, the splice and the allow-list entry
// this run already wrote (writer.go, applied twice by design.md's rollback
// requirement).
func (coverageTiersStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	plan := planCoverageTiers(p)
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

	// ensureShared, and not a rewrite of the ignore file: it appends the entry
	// the repository is missing and leaves every other line — the template, and
	// whatever the project added for itself — byte for byte.
	for _, name := range plan.shared {
		if err := ensureShared(p, w, name); err != nil {
			return Facts{}, err
		}
	}
	return Facts{}, nil
}

// coverageTiersFile is one file this step owns, with the exact bytes it writes.
type coverageTiersFile struct {
	path string
	data []byte
}

// coverageTiersPlan is everything the step derives, computed once per call so
// Satisfied, Describe, Delegated and Apply cannot disagree about it (§07).
//
// conflict and files are mutually exclusive: a refusal is the whole answer, and
// a plan that says "dharness will not write this" never also says "here are the
// bytes" (§20 — a blocked step descends a rung, it does not half-write).
//
// shared is what a written plan still needs from the repository: the allow-list
// entries that make the generated modules survive the checkout boundary. It is
// part of satisfies, not a side note, because a plan whose bytes already match
// while the allow list does not is exactly the state that reads as done and
// still loses the files the gate runs against on the next clone.
type coverageTiersPlan struct {
	conflict  string
	files     []coverageTiersFile
	gitignore string
	shared    []string
}

// satisfies reports whether every byte this plan would write is already on
// disk, which is the whole of this step's idempotence: the canonical bytes are
// read back, never remembered.
func (plan coverageTiersPlan) satisfies() bool {
	for _, file := range plan.files {
		existing, err := os.ReadFile(file.path)
		if err != nil || !bytes.Equal(existing, file.data) {
			return false
		}
	}
	if len(plan.shared) == 0 {
		return true
	}

	raw, err := os.ReadFile(plan.gitignore)
	if err != nil {
		return false
	}
	body := string(raw)
	for _, name := range plan.shared {
		if !strings.Contains(body, "!"+name) {
			return false
		}
	}
	return true
}

// planCoverageTiers derives this repository's coverage wiring, or the reason
// there is none.
//
// The order is §12's ascending cost, and the first answer wins: the manifest
// and its declaration (one read, one parse), the runner's presence (one stat),
// the config's presence (one stat per candidate name), the resolver (one walk
// of the adopted prefixes) and only then the splice, which reads and parses.
// Nothing here writes, starts a process or installs anything.
func planCoverageTiers(p project.Project) coverageTiersPlan {
	if !coverageTiersDeclared(p) {
		if reason := coverageTiersLeftBehind(p); reason != "" {
			return coverageTiersPlan{conflict: reason}
		}
		if p.HasSource() {
			return coverageTiersPlan{conflict: coverageTiersAdoptionPrompt(p)}
		}
		return coverageTiersPlan{}
	}

	if !p.HasSource() {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s declares this repository's coverage tiers, but there is no JS project here for them\nto be enforced in. Add the JS project, or remove the declaration.",
			tiers.ManifestName)}
	}

	// The resolver is the declaration's authority, so an invalid one is
	// reported before anything dharness might otherwise say about this
	// project. Its own message already names the declaration and the fix; the
	// resolved file list itself is not read here — the runtime asks the
	// resolver again at load time, where the answer is used.
	if _, err := tiers.Resolve(p.Root); err != nil {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s\ndharness writes nothing until the declaration resolves; the gate keeps asking the\nresolver, so this is not a state to commit in.", err)}
	}

	if p.LocalBinary(tool.Vitest) == "" {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s declares this repository's coverage tiers, but the runner they are enforced with is\nnot installed in this project, so the gate would have nothing to run. Install it\nwhere the project installs things:\n\n    %s %s %s\n\ndharness never downloads it at sync time.",
			tiers.ManifestName, tool.InstallCommand(p.PackageManager), tool.Vitest, "@vitest/coverage-v8")}
	}

	configPath := coverageTiersNativeConfig(p)
	if configPath == "" {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s declares this repository's coverage tiers, but this project has no Vitest config to\ncarry them: dharness points the project's own config at the managed runtime and does\nnot write a test config of its own. Add %s (or a %s Vitest reads), then run\n`dharness sync` again.",
			tiers.ManifestName, coverageVitestConfigNames[0], coverageViteConfigNames[0])}
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s declares this repository's coverage tiers, and %s could not be read: %v",
			tiers.ManifestName, filepath.Base(configPath), err)}
	}

	candidate, err := spliceCoverageTiersConfig(raw, configPath, p)
	if err != nil {
		return coverageTiersPlan{conflict: fmt.Sprintf(
			"%s declares this repository's coverage tiers, and %s cannot carry the managed\nreferences without guessing at the project's own code:\n\n%s\n\nFix the config (or the declaration), then run `dharness sync` again; until then the\nmanaged regions are not written and nothing the project owns is touched.",
			tiers.ManifestName, filepath.Base(configPath), err)}
	}

	bridge, err := coverageBridgeModule(configPath, p)
	if err != nil {
		return coverageTiersPlan{conflict: err.Error()}
	}

	files := []coverageTiersFile{
		{path: filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName), data: coverageTiersModule()},
		{path: filepath.Join(p.Root, project.Dir, coverageBridgeName), data: bridge},
		{path: configPath, data: candidate},
		// The gate file converges here rather than in ownedFilesStep, which
		// writes it from the repository state at step 2 — before this step's
		// splice exists, so a first sync would otherwise leave the coverage
		// line for a later run. The bytes come from the same renderer, so the
		// two writers cannot disagree.
		{path: filepath.Join(p.Root, project.Dir, ownedLefthook), data: []byte(gateConfigWithCoverage(p, coverageTiersGateCommand(p)))},
	}

	if hookManager(p) == managerHusky {
		hook, err := coverageTiersHuskyHook(p)
		if err != nil {
			return coverageTiersPlan{conflict: err.Error()}
		}
		// nil means there is no hook file yet: the check line belongs to
		// hookInstallStep, and this step never writes it.
		if hook != nil {
			files = append(files, coverageTiersFile{path: filepath.Join(p.Root, filepath.FromSlash(huskyHook)), data: hook})
		}
	}

	return coverageTiersPlan{
		files:     files,
		gitignore: filepath.Join(p.Root, project.Dir, ownedIgnore),
		shared:    []string{coverageTiersRuntimeName, coverageBridgeName},
	}
}

// coverageTiersAdoptionPrompt offers a project decision, not an implicit opt-in.
// It is the delegated report's executable instruction for both human and JSON
// readers; guidance must be available before the declaration unlocks the skill.
func coverageTiersAdoptionPrompt(p project.Project) string {
	source := p.SourceRel()
	if source == "" {
		source = "."
	}
	return fmt.Sprintf(`coverage-tiers.json is absent: coverage tiers are optional and not enforced.
Offer adoption to the user; do not treat absence as completed enforcement or
silently choose the project's business policy. If adoption is deferred, leave
coverage files and the gate unchanged; sync will offer this choice again.

If adoption is approved:
1. Inspect the existing source and tests. Agree on the feature scope and its
   business impact: CORE 100%%, IMPORTANT 80%%, INFRASTRUCTURE 0%%. These are native
   coverage targets, not mutation-score thresholds; infrastructure is excluded
   from mutation. Do not lower tiers or exclude behavior just to pass.

2. Create coverage-tiers.json at the repository root with version: 1,
   enforcedPrefixes, and features containing id, path, defaultTier and reason
   (optional overrides contain path, tier and reason). Paths are repository-relative
   with forward slashes; directory prefixes end in /. Adopt only existing paths,
   starting narrowly: all source under enforcedPrefixes must belong to a feature,
   including its older files. Leave not-yet-created paths out until they exist.

3. Native integration supports Vitest only, not Jest or Go coverage. In %s,
   install missing local prerequisites with the project's package manager:

       %s vitest @vitest/coverage-v8

   Omit already-declared packages from that add command; restore them with the
   package manager's install command instead. Preserve declared versions and
   choose compatible runner/provider versions. dharness sync does not install them. Supply the project's own Vitest/Vite
   config and any test environment or UI-test dependencies the project needs;
   dharness does not create that config or write the tests.

4. Validate with dharness policy --root . from the repository root, then run
   dharness sync. It writes the managed coverage runtime and checked bridge,
   splices the native config, adds the local-hook coverage job and distributes
   the coverage-tiers skill with references in existing AGENTS.md/CLAUDE.md.
   Read that skill, close real test gaps, and run the exact native coverage
   command in .dharness/lefthook.yml or the husky hook from the JS project root.
   Verify the complete adopted feature, not just changed files. sync does not
   run the tests, add package scripts such as test:coverage, wire an existing
   check script, or create CI workflows; offer those separately when needed.`,
		source, tool.InstallCommand(p.PackageManager))
}

// coverageTiersDeclared reports whether this repository opted in at all. The
// stat is the whole question: everything else in this file is derived from the
// declaration's own answer.
func coverageTiersDeclared(p project.Project) bool {
	if p.Root == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(p.Root, tiers.ManifestName))
	return err == nil
}

// coverageTiersNativeConfig names the project's own Vitest config, or "" when
// it has none — the same "which one responds" question fallowConfigPath and
// eslintFlatConfig already answer for their files.
func coverageTiersNativeConfig(p project.Project) string {
	if !p.HasSource() {
		return ""
	}
	if path := firstExisting(p.Source, coverageVitestConfigNames); path != "" {
		return path
	}
	return firstExisting(p.Source, coverageViteConfigNames)
}

// coverageTiersWired reports whether the project's own config already carries
// the managed regions. It reads the markers rather than the whole splice: the
// question the gate turns on is "was this config ever pointed at the managed
// runtime", and markers it wrote are that answer.
func coverageTiersWired(p project.Project) bool {
	path := coverageTiersNativeConfig(p)
	if path == "" {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return coverageTiersMarkersPresent(string(raw))
}

// coverageTiersMarkersPresent reports whether raw carries exactly one
// well-formed region of every marker pair this product owns. Malformed counts
// as absent here: the splicer refuses that state, so it is not the wiring the
// gate line was waiting for.
func coverageTiersMarkersPresent(raw string) bool {
	for _, pair := range coverageTiersMarkerPairs() {
		if locateCoverageTiersRegion(raw, pair.begin, pair.end).state != markersPresent {
			return false
		}
	}
	return true
}

// coverageTiersMarkerPairs are the three pairs the coverage adapter owns, as
// one list, so the presence test above and the bridge's own guard are rendered
// from the same names — a marker renamed in one place cannot leave the other
// answering about a string that no longer exists.
func coverageTiersMarkerPairs() []struct {
	begin string
	end   string
} {
	return []struct {
		begin string
		end   string
	}{
		{coverageTiersImportBegin, coverageTiersImportEnd},
		{coverageTiersExportBegin, coverageTiersExportEnd},
		{coverageTiersInputBegin, coverageTiersInputEnd},
	}
}

// coverageTiersLeftBehind names the bytes this step owns that survive a removed
// declaration, or "".
//
// It is a real conflict and not a cleanup: `dharness policy` refuses to answer
// while the managed module exists without the manifest that names it, because
// an empty projection is a gate that measures nothing and says it passed. So
// the state is reported with both directions out of it, and nothing is deleted
// — whether the declaration is coming back is the project's decision, not
// dharness's (§05, §20).
func coverageTiersLeftBehind(p project.Project) string {
	found := coverageTiersFootprint(p)
	if len(found) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"there is no %s, but the managed coverage integration it activated is still here:\n%s.\n\n`dharness policy` refuses to answer in this state — the managed module would resolve\nnothing while the gate still asks it to — so this is a real conflict rather than\nleftovers. Restore %s, or remove the managed file(s) and the marked regions to drop\nthe adoption.",
		tiers.ManifestName, strings.Join(found, "\n  "), tiers.ManifestName)
}

// coverageTiersHuskyMarked reports whether the husky hook already carries a line
// this step owns.
func coverageTiersHuskyMarked(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
	return err == nil && strings.Contains(string(raw), coverageHuskyMarker)
}

// coverageTiersActivated reports whether this repository is on the coverage
// hook at all: its declaration, or any byte of the integration dharness has
// already written.
//
// The footprint is what makes the gate fail closed rather than follow the
// declaration around. A manifest that was deleted under a live integration, a
// declaration that cannot be prepared, a config whose splice is refused: none
// of those is a reason to take the native coverage job out of the gate, because
// the bytes it names are still there and the loader refuses what they point at.
// Dropping the job would turn every one of those states into a green commit
// that measured nothing.
func coverageTiersActivated(p project.Project) bool {
	return coverageTiersDeclared(p) || len(coverageTiersFootprint(p)) > 0
}

// coverageTiersFootprint names the managed bytes of this step that are present,
// relative to the repository root and in a stable order, or nil when there are
// none.
//
// Every one of them is read, never remembered (§07): the module and the bridge
// are files dharness owns, the managed regions are the reference it wrote into
// a file the project owns, and the husky marker is the line it added to a hook
// that is the project's too. A repository with none of them and no declaration
// is a repository dharness has never touched.
func coverageTiersFootprint(p project.Project) []string {
	// A project with no root has no directory to read, and joining onto an
	// empty root would stat this process's own working directory — the unsafe
	// read boundariesOwnerStep already guards its own equivalent against.
	if p.Root == "" {
		return nil
	}

	var found []string
	for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
		if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
			found = append(found, filepath.ToSlash(filepath.Join(project.Dir, name)))
		}
	}
	if path := coverageTiersNativeConfig(p); path != "" && coverageTiersWired(p) {
		display := filepath.Base(path)
		if rel, err := filepath.Rel(p.Root, path); err == nil {
			display = filepath.ToSlash(rel)
		}
		found = append(found, display)
	}
	if hookManager(p) == managerHusky && coverageTiersHuskyMarked(p) {
		found = append(found, huskyHook)
	}
	return found
}

// coverageTiersGateCommand is the line lefthook runs after the check. lefthook
// changes directory per command — the `root:` key gateConfig already emits —
// so the line itself carries no directory change, and the shell is whatever
// lefthook hands a job to on this machine.
func coverageTiersGateCommand(p project.Project) string {
	return coverageTiersCommandFor(p, "", tool.HostCoverageShell())
}

// coverageTiersHuskyCommand is the same line for a husky hook, which is a POSIX
// shell script on every platform: naming the host's shell here is exactly the
// mistake that would hand a Windows machine's hook a cmd.exe line.
func coverageTiersHuskyCommand(p project.Project) string {
	return coverageTiersCommandFor(p, p.SourceRel(), tool.ShellPOSIX)
}

// coverageTiersCommandFor renders the gate's coverage job for a hook whose
// working directory is runDir, or "" when this repository never activated the
// tiers.
//
// An activated repository always gets a job. When the line itself cannot be
// rendered — no JS project root to name the entry from, or a value the shell
// would rewrite — the job is a refusal that fails by name, never an omission:
// a gate that quietly loses its coverage step passes exactly the commits the
// policy exists to stop.
func coverageTiersCommandFor(p project.Project, runDir string, shell tool.CoverageShell) string {
	if !coverageTiersActivated(p) {
		return ""
	}

	line, err := coverageTiersLineFor(p, runDir, shell)
	if err != nil {
		return tool.CoverageRefusal(shell, coverageTiersRefusal(err))
	}
	return line
}

// coverageTiersLineFor renders the invocation itself: the project's manager, in
// its local-only form, running the project's own Vitest with coverage against
// the entry dharness owns, named relative to the JS project root because that
// is where vitest runs.
func coverageTiersLineFor(p project.Project, runDir string, shell tool.CoverageShell) (string, error) {
	entry, err := coverageTiersEntryFromSource(p)
	if err != nil {
		return "", err
	}
	return tool.VitestCoverageCommand(shell, p.PackageManager, runDir, entry)
}

// coverageTiersEntryFromSource names the bridge the gate runs against, the way the JS
// project root sees it.
//
// It is the one reference this step needs that a project can genuinely lack:
// with no JS project there is no directory to name it from, which is a boundary
// rather than a licence to unwire the gate.
func coverageTiersEntryFromSource(p project.Project) (string, error) {
	if !p.HasSource() {
		return "", errors.New("this repository activated the coverage tiers, but there is no JS project root to name the managed entry from")
	}
	rel, err := filepath.Rel(p.Source, filepath.Join(p.Root, project.Dir, coverageBridgeName))
	if err != nil {
		return "", fmt.Errorf(
			"the JS project root %s and the managed entry %s share no relative path (%v), and dharness writes a relative reference so a moved checkout keeps resolving",
			p.Source, filepath.ToSlash(filepath.Join(p.Root, project.Dir, coverageBridgeName)), err)
	}
	return filepath.ToSlash(rel), nil
}

// coverageTiersRefusal states why the coverage job could not be rendered, in
// the words the gate will print when it fails. The cause already names the
// boundary; this adds the one thing a reader of a failed commit needs, which is
// what to do about it.
func coverageTiersRefusal(cause error) string {
	return fmt.Sprintf("%s; run dharness sync from the project root and fix what it reports", cause)
}

// huskyGateLine is the one line the husky hook runs for this repository: the
// check by itself when nothing activated the tiers, and the check chained with
// the coverage run — or the refusal that stands in for it — when the
// declaration or the managed footprint did.
//
// It is the single renderer hookInstallStep and coverageTiersStep both write,
// so an activated repository cannot end a run with a hook that runs only the
// check: `dharness check` exits 0 with nothing staged, which is exactly the
// silent green the coverage job exists to prevent.
func huskyGateLine(p project.Project) string {
	coverage := coverageTiersHuskyCommand(p)
	if coverage == "" {
		return gateCommand
	}
	return gateCommand + " && " + coverage + " " + coverageHuskyMarker
}

// convergeHuskyGate returns body with the line this repository's gate runs, and
// whether body carried a check line to converge onto.
//
// It is one line rather than two appended ones because of what a shell script
// does with a failing command: husky's hook is not run with `set -e`, so a
// `dharness check` that fails on its own line would be followed by the coverage
// run, and the script's exit status would be coverage's — a failed gate whose
// coverage happened to pass would let the commit through. `&&` is the ordering
// and the verdict in one token.
//
// The hook is the project's own file and stays one: the line is identified by
// its own marker and by the check it carries, so a project that changed package
// manager or moved its JS root has the stale line replaced rather than joined by
// a second one, and every other line — husky's own header, the project's
// commands — is carried through untouched.
func convergeHuskyGate(p project.Project, body string) (string, bool) {
	pieces := strings.SplitAfter(body, "\n")

	owned, check := -1, -1
	for i, piece := range pieces {
		if strings.Contains(piece, coverageHuskyMarker) && owned == -1 {
			owned = i
		}
		if strings.Contains(piece, gateCommand) && check == -1 {
			check = i
		}
	}
	if check == -1 {
		return "", false
	}

	line := huskyGateLine(p)
	var out strings.Builder
	for i, piece := range pieces {
		if piece == "" {
			continue
		}
		switch {
		case i == check:
			out.WriteString(line)
			out.WriteString(regionLineEnding(piece, 0))
		case i == owned:
			continue
		default:
			out.WriteString(piece)
		}
	}
	return out.String(), true
}

// coverageTiersHuskyHook returns the husky hook's bytes with the coverage run
// chained onto the check line, or nil when there is no check line to chain it
// to — and nil when nothing activated the tiers, because a hook that runs only
// the check is already what this repository's own state renders.
func coverageTiersHuskyHook(p project.Project) ([]byte, error) {
	path := filepath.Join(p.Root, filepath.FromSlash(huskyHook))
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", huskyHook, err)
	}

	if coverageTiersHuskyCommand(p) == "" {
		return nil, nil
	}
	converged, ok := convergeHuskyGate(p, string(existing))
	if !ok {
		return nil, nil
	}
	return []byte(converged), nil
}

// coverageBridgeModule renders the entry the native run is pointed at.
//
// It is a bridge and nothing else: it re-exports the project's own default and
// refuses to load when that config never gained the managed regions. The refusal
// is the whole reason the gate names this file rather than the project's config
// — a gate pointed straight at an unwired config would run coverage with nothing
// to enforce and exit 0, which is the silent green the tiers exist to prevent.
// Nothing is classified, merged or resolved here; that is the managed runtime's
// job, at load, from the Go resolver (§17).
func coverageBridgeModule(configPath string, p project.Project) ([]byte, error) {
	if p.Root == "" {
		return nil, fmt.Errorf(
			"coverage: this project has no repository root, so the bridge entry %s has no directory\nto be written in", coverageBridgeName)
	}

	rel, err := filepath.Rel(filepath.Join(p.Root, project.Dir), configPath)
	if err != nil {
		return nil, fmt.Errorf(
			"coverage: %s and %s share no relative path (%v); dharness writes a relative import so a moved\ncheckout keeps resolving",
			coverageBridgeName, filepath.Base(configPath), err)
	}
	specifier := moduleSpecifier(filepath.ToSlash(rel))

	var b strings.Builder
	b.WriteString("// Managed by dharness. Do not edit: the next `dharness sync` writes this file\n")
	b.WriteString("// again from the binary it ships in.\n")
	b.WriteString("//\n")
	b.WriteString("// The gate runs the project's own Vitest against this entry rather than against\n")
	fmt.Fprintf(&b, "// %s directly. This file is a bridge and nothing else: it\n", filepath.Base(configPath))
	b.WriteString("// re-exports that config's own default, and it refuses to load when the config\n")
	b.WriteString("// never gained the managed regions. That refusal is the point: a gate pointed\n")
	b.WriteString("// straight at an unwired config would run coverage with nothing to enforce and\n")
	b.WriteString("// exit 0. The tiers belong to ")
	b.WriteString(coverageTiersRuntimeName)
	b.WriteString(", resolved from the project's own ")
	b.WriteString(tiers.ManifestName)
	b.WriteString("; nothing is classified,\n// merged or evaluated here.\n\n")
	b.WriteString("import { readFileSync } from \"node:fs\";\n")
	b.WriteString("import { fileURLToPath } from \"node:url\";\n\n")
	fmt.Fprintf(&b, "import consumerConfig from %q;\n\n", specifier)
	fmt.Fprintf(&b, "const consumerPath = fileURLToPath(new URL(%q, import.meta.url));\n", specifier)
	b.WriteString("const consumerSource = readFileSync(consumerPath, \"utf8\");\n\n")
	b.WriteString("const missing = [\n")
	for _, pair := range coverageTiersMarkerPairs() {
		fmt.Fprintf(&b, "  %q,\n", pair.begin)
	}
	b.WriteString("].filter((marker) => !consumerSource.includes(marker));\n\n")
	b.WriteString("if (missing.length > 0) {\n")
	b.WriteString("  throw new Error(\n")
	b.WriteString("    \"coverage tiers: \" + consumerPath + \" does not carry the managed regions (\" + missing.join(\", \")\n")
	b.WriteString("      + \"), so this run would measure nothing the declared tiers require; run `dharness sync` to wire it\",\n")
	b.WriteString("  );\n")
	b.WriteString("}\n\n")
	b.WriteString("export default consumerConfig;\n")
	return []byte(b.String()), nil
}
