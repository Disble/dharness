package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/jsconfig"
	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// gateCommand is the single line every hook manager ends up invoking, which is
// what keeps lefthook and husky from becoming a variable anywhere else.
const gateCommand = "dharness check"

// gateConfig is the lefthook configuration dharness owns.
//
// The `root` key is lefthook's own, and it is the whole reason dharness does
// not have to solve this itself: the hook is installed at the repository root
// because that is where git looks for it, but in a split layout the tools have
// to run where the package manager installed them. lefthook already changes
// directory per command, so dharness names the directory and stops there.
//
// It is emitted only when the two roots really differ. A key that says `root:
// ./` in a conventional repository is noise in a file people read.
func gateConfig(p project.Project) string {
	gate := fmt.Sprintf("pre-commit:\n  commands:\n    dharness:\n      run: %s\n", gateCommand)
	if rel := p.SourceRel(); rel != "" {
		gate += fmt.Sprintf("      root: %s/\n", rel)
	}
	return gate
}

// ---------------------------------------------------------------- install

type installStep struct{}

func (installStep) ID() string { return "install what this project is missing" }

// Satisfied answers the only question dharness can answer on its own: is there
// a JS project here at all. Whether the package is already there is the
// package manager's question, and every way of asking it from outside is
// wrong. A directory under node_modules is an install artifact — it survives a
// rollback that restored package.json, and Yarn PnP and pnpm's store often
// leave none at all. A name in package.json is a declaration nobody has
// necessarily acted on. Reading a lockfile means parsing four formats to
// re-derive an answer the install command returns for free.
//
// So dharness does not ask. It runs the install, which is idempotent in all
// four managers, and the exit code is the verdict.
func (installStep) Satisfied(p project.Project) bool {
	return !p.HasSource()
}

func (s installStep) Describe(p project.Project) string {
	return fmt.Sprintf("This package provides dharness's project lint rules.\n\n    %s %s",
		tool.InstallCommand(p.PackageManager), strings.Join(integrationPackages(p), " "))
}

// Delegated is always false: there is no repository state that hands
// installing a package to the agent instead of dharness.
func (installStep) Delegated(project.Project) (string, bool) { return "", false }

// Apply routes the install and its rollback compensation through out rather
// than os.Stdout/os.Stderr directly (step-outcome's sink requirement,
// defect 5), and returns the package specs this run asked the manager to
// add as Facts — a fact no byte stream can answer (Decision 2).
func (installStep) Apply(p project.Project, w *Writer, out io.Writer) (Facts, error) {
	packages := integrationPackages(p)

	for _, path := range p.PackageStateFiles() {
		if err := w.remember(path); err != nil {
			return Facts{}, fmt.Errorf("snapshot package state before install: %w", err)
		}
	}

	installErr := runner.Run(tool.InstallPackages(p.PackageManager, p.Source, packages), out, out)
	w.compensate(func() error {
		if err := runner.Run(tool.RemovePackages(p.PackageManager, p.Source, packages), out, out); err != nil {
			return fmt.Errorf("remove integration packages added by this run: %w", err)
		}
		return nil
	})
	if installErr != nil {
		return Facts{}, installErr
	}
	return Facts{Installed: installedWithVersions(p, packages)}, nil
}

// installedWithVersions names each installed package the way a reader can
// act on: "name@version", not the bare name Facts.Installed named before —
// "installed dharness-eslint-plugin" says nothing about what actually
// landed (gap 3, from the team lead's measured run). The version is read
// back from package.json after the install this same call just ran, never
// invented: a package this run added but package.json does not (yet)
// resolve — an unreadable file, a manager that writes it differently, a
// name this scan does not find — falls back to its bare name rather than a
// fabricated version, matching the "reported whole or not at all" rule
// config-collision's own requirement states for a colliding value.
//
// This is a disclosed, deliberate departure from design.md Decision 2
// ("Facts.Installed names what dharness asked for, no version"): that
// decision reasoned from what Apply could answer from its own inputs alone.
// The measured gap shows a reader needs more than that, and package.json is
// already read back after every install for exactly this purpose. The
// caveat design.md's own reasoning correctly anticipated: package.json may
// record a caret/tilde range rather than the exact resolved version,
// depending on the project's own package-manager settings — this reports
// whatever range or exact version package.json actually carries, stripped
// of the range operator, never a version measured from a lockfile (which
// differs by manager and is out of scope here per CLAUDE.md's "if the CLI
// already does it, dharness does not do it").
func installedWithVersions(p project.Project, packages []string) []string {
	raw, err := os.ReadFile(filepath.Join(p.Source, "package.json"))
	if err != nil {
		return packages
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return packages
	}

	versioned := make([]string, len(packages))
	for i, name := range packages {
		// !ok is a proven equivalent condition to version == "" here, not
		// tested separately (mutation-tdd: disable only a proven
		// equivalent mutant, at the narrowest location, with a written
		// reason): Go's comma-ok map lookup always yields the zero value
		// ("") for version when a lookup misses, on both the
		// DevDependencies and (if that also misses) Dependencies try, so
		// !ok can never be true while version is non-empty. The `!ok ||`
		// term is kept for readability — "not found, or found empty" — not
		// because it changes behaviour in any reachable state.
		version, ok := pkg.DevDependencies[name]
		if !ok {
			version, ok = pkg.Dependencies[name]
		}
		if !ok || version == "" {
			versioned[i] = name
			continue
		}
		versioned[i] = name + "@" + strings.TrimLeft(version, "^~")
	}
	return versioned
}

// integrationPackages lists the packages dharness adds to p, as opposed to
// the CLIs it invokes without installing: the fixed set every project gets,
// plus whatever the matched presets contribute. It takes a project now
// because the answer depends on detection — the same re-derive-at-the-
// call-site rule framework-presets decision 4 fixed, and for the same
// reason: a memoised value would be recorded state (§07).
func integrationPackages(p project.Project) []string {
	packages := []string{RulesPackage}
	for _, layer := range preset.Layers(preset.Resolve(p)) {
		packages = append(packages, layer.InstallName())
	}
	return dedupe(packages)
}

// ---------------------------------------------------------- owned files

type ownedFilesStep struct{}

func (ownedFilesStep) ID() string { return "write the files dharness owns" }

// Satisfied checks lefthook.yml and rules.json for existence, as before —
// dharness owns both outright and rewrites them wholesale every run, so
// existence is the whole question. fallow.jsonc is different: it is
// co-owned with the agent's `boundaries` block, so only the delimited
// region's bytes are compared against what the currently matched presets
// would render (design decision 8) — the rest of the file, boundaries
// included, is never read here.
func (ownedFilesStep) Satisfied(p project.Project) bool {
	for _, name := range []string{ownedLefthook, ownedRules} {
		if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err != nil {
			return false
		}
	}

	matches := preset.Resolve(p)

	raw, err := os.ReadFile(filepath.Join(p.Root, project.Dir, ownedFallow))
	if err != nil {
		return false
	}
	if regionBytes(string(raw)) != presetRegion(matches) {
		return false
	}

	// Unlike fallow.jsonc, eslint.config.js carries no agent-editable block —
	// dharness wrote every byte of it — so the whole file is compared, the
	// same way the six severities converge on the next run rather than
	// being written once (design decision 8). The read error is not checked
	// separately: ownedEslintConfig never renders "", so a missing file
	// (read back as empty bytes) can never equal it either way.
	module := projectEslintModule(p)
	eslint, _ := os.ReadFile(filepath.Join(p.Root, project.Dir, ownedEslintName(module)))
	if string(eslint) != ownedEslintConfig(p, preset.Layers(matches), module) {
		return false
	}

	// The allow list is repaired rather than written once (design decision
	// 2): a repository adopted before this change predates the entry, and
	// hand-removing it must bring the step back the same way any other
	// derived state does (§15). Same reasoning as above: a missing
	// .gitignore reads back as empty bytes, which never contains the
	// non-empty entry string either.
	ignore, _ := os.ReadFile(filepath.Join(p.Root, project.Dir, ".gitignore"))
	return strings.Contains(string(ignore), "!"+ownedEslintName(module))
}

func (ownedFilesStep) Describe(project.Project) string {
	return fmt.Sprintf("The gate, the architecture and the rule thresholds live in %s/, which is\ncommitted. The project's own files only ever gain one line pointing at them.",
		project.Dir)
}

// Delegated is always false: the files dharness owns are always dharness's to
// write.
func (ownedFilesStep) Delegated(project.Project) (string, bool) { return "", false }

func (ownedFilesStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	// EnsureDir also writes the ignore rules, so a transient file appearing
	// later is never the first thing to create them.
	if _, err := p.EnsureDir(""); err != nil {
		return Facts{}, err
	}

	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedLefthook), []byte(gateConfig(p))); err != nil {
		return Facts{}, err
	}

	// The boundaries block is deliberately absent from what dharness ever
	// asserts here: zones encode intent, and no detection can read intent off
	// a tree, so the model fills it in. It survives every later run because
	// only the delimited region below is rewritten, never the whole file
	// (design decision 8) — fallow.jsonc is the one owned file the agent also
	// writes into, and rewriting it wholesale would destroy that block the
	// moment a matched preset's contribution changed.
	fallowPath := filepath.Join(p.Root, project.Dir, ownedFallow)
	base := architectureSkeleton
	if existing, err := os.ReadFile(fallowPath); err == nil {
		base = string(existing)
	}
	matches := preset.Resolve(p)
	content := replaceRegion(base, presetRegion(matches))
	if err := w.Write(fallowPath, []byte(content)); err != nil {
		return Facts{}, err
	}

	module := projectEslintModule(p)
	eslintPath := filepath.Join(p.Root, project.Dir, ownedEslintName(module))
	if err := w.Write(eslintPath, []byte(ownedEslintConfig(p, preset.Layers(matches), module))); err != nil {
		return Facts{}, err
	}
	if err := ensureShared(p, w, ownedEslintName(module)); err != nil {
		return Facts{}, err
	}

	// A project adopted before the name carried its dialect has an
	// eslint.config.js here that nothing imports any more: the reference in
	// its own config lives in a marked region, which this same run rewrites
	// to the new name. Remembering it first is what puts the removal inside
	// the run's transaction — Writer.Undo restores a file it snapshotted,
	// whether the step deleted it or overwrote it.
	legacy := filepath.Join(p.Root, project.Dir, ownedEslintLegacyName)
	if _, err := os.Stat(legacy); err == nil {
		if err := w.remember(legacy); err != nil {
			return Facts{}, err
		}
		if err := os.Remove(legacy); err != nil {
			return Facts{}, fmt.Errorf("remove the eslint config an earlier dharness owned: %w", err)
		}
	}

	return Facts{}, w.WriteJSON(filepath.Join(p.Root, project.Dir, ownedRules), DefaultThresholds())
}

// ------------------------------------------------------------- extends
//
// fallowExtendsStep and lefthookExtendsStep are split rather than one step
// with two targets, because the two targets can have two different
// recipients: the project owns .fallowrc.json but may have no lefthook.yml
// at all, and a single Delegated answer cannot speak for both files at once.

type fallowExtendsStep struct{}

func (fallowExtendsStep) ID() string {
	return fmt.Sprintf("point %s at the file dharness owns", fallowConfig)
}

func (fallowExtendsStep) Satisfied(p project.Project) bool {
	return !p.HasSource() || extendsWired(p.Source, fallowConfig, ownedFrom(p, p.Source, ownedFallow))
}

func (fallowExtendsStep) Describe(p project.Project) string {
	target := ownedFrom(p, p.Source, ownedFallow)
	return fmt.Sprintf(
		"fallow composes with `extends`, so the architecture arrives by reference.\nAdd this line to %s:\n\n    \"extends\": [%q]",
		fallowConfig, target)
}

// Delegated returns ok == true only when the project's own config already
// exists: adding a key to it is then a merge, not a write dharness gets to
// make on its own. With no config present, dharness writes the whole file.
func (fallowExtendsStep) Delegated(p project.Project) (string, bool) {
	if !p.HasSource() {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(p.Source, fallowConfig)); errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return fmt.Sprintf(
		"%s already exists and belongs to the project; adding a key to it is a merge,\nnot a write.",
		fallowConfig), true
}

func (fallowExtendsStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	return Facts{}, wireFallowExtends(p, w)
}

type lefthookExtendsStep struct{}

func (lefthookExtendsStep) ID() string {
	return fmt.Sprintf("point %s at the file dharness owns", lefthookConfig)
}

func (lefthookExtendsStep) Satisfied(p project.Project) bool {
	return hookManager(p) != managerLefthook ||
		extendsWired(p.Root, lefthookConfig, ownedFrom(p, p.Root, ownedLefthook))
}

func (lefthookExtendsStep) Describe(p project.Project) string {
	target := ownedFrom(p, p.Root, ownedLefthook)
	return fmt.Sprintf(
		"lefthook composes with `extends`, so the gate arrives by reference.\nAdd this line to %s:\n\n    extends:\n      - %s",
		lefthookConfig, target)
}

// Delegated follows the same rule as fallowExtendsStep, for the project's own
// lefthook.yml instead of .fallowrc.json — with one exception fallow has no
// equivalent of.
//
// `lefthook install` writes a config when it finds none: an example file with
// every key commented out. A repository that took step 9's advice and set a
// hook manager up by hand arrives at the next sync carrying one. Refusing it
// as "belonging to the project" was refusing a file dharness's own delegation
// caused to exist, and left this step delegated on every run from then on,
// with nothing a user could do to clear it short of deleting the file.
//
// A config with no uncommented key is not a decision. There is nothing to
// merge with and nothing to lose, so dharness writes.
func (lefthookExtendsStep) Delegated(p project.Project) (string, bool) {
	if hookManager(p) != managerLefthook {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err == nil && declaresNothing(raw) {
		return "", false
	}
	// A config already carrying the reference has nothing left to delegate.
	// Satisfied answers true for it too, and run() asks Satisfied first, so
	// this branch used to be unreachable — until hookInstallStep started
	// asking this question directly to decide whether it is waiting on the
	// same merge. Both answering true is the pair run()'s own contract
	// forbids.
	if extendsWired(p.Root, lefthookConfig, ownedFrom(p, p.Root, ownedLefthook)) {
		return "", false
	}
	return fmt.Sprintf(
		"%s already exists and belongs to the project; adding a key to it is a merge,\nnot a write.",
		lefthookConfig), true
}

// declaresNothing reports whether raw is comments and blank lines all the way
// down — lefthook's untouched scaffold, or an empty file.
//
// The test is textual because that is the whole of the question: one line
// that is not a comment is a key somebody wrote, whatever it says. A YAML
// parser would answer the same thing at the cost of a grammar dharness does
// not otherwise need, and "#" cannot start a YAML key, so there is no shape
// this reads backwards.
func declaresNothing(raw []byte) bool {
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return false
	}
	return true
}

func (lefthookExtendsStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	return Facts{}, wireLefthookExtends(p, w)
}

// -------------------------------------------------------- eslint extends
//
// eslintExtendsStep is a third member of the family above, for
// eslint.config.js instead of .fallowrc.json or lefthook.yml — split out
// for the same reason that pair already is: the file it targets is neither
// of theirs.
//
// Unlike the other two, presence alone does not delegate (the MODIFIED
// step-delegation requirement in spec.md): where the existing config is
// something dharness understands well enough to edit, Delegated answers
// ok == false and Apply edits it directly — inserting the two marked regions
// when neither exists yet, replacing them in place when they already do
// (design decision 6). Satisfied is a byte comparison, the same rule
// ownedFilesStep.Satisfied already applies to a file dharness owns: true
// when the marked regions are absent for a reason that always delegates (no
// flat config splice is eligible for), or when they are present and equal to
// what this run would render. A config spliced before a preset started
// contributing a layer converges by replacement on the next run instead of
// drifting.

type eslintExtendsStep struct{}

func (eslintExtendsStep) ID() string {
	return fmt.Sprintf("point %s at the file dharness owns", eslintConfig)
}

// Satisfied answers a byte comparison for a splice-eligible flat config
// (design decision 6, ownedFilesStep.Satisfied's rule applied to a file
// dharness does not own): true only when the candidate this run would
// produce is byte-identical to what is already on disk.
//
// A shape that delegates is never satisfied. That is the opposite of what
// this answered before, and the reason is run()'s own order: it asks
// Satisfied first and skips a satisfied step without ever asking Delegated,
// so a step answering true to both has its reason computed and discarded —
// an unwired ESLint layer filed under "Already in place" while the plugins
// step 1 just installed do nothing. The old rule ("Delegated already
// explains it, so Satisfied should not double-report the same state as
// pending") was written against Pending/applySteps, where Satisfied selects
// and Delegated skips; under run() it is not a double report, it is a
// silent one.
//
// Answering false costs nothing on the apply path: Pending gains the entry,
// and applySteps skips every step whose Delegated answers ok == true before
// it can reach Apply.
func (s eslintExtendsStep) Satisfied(p project.Project) bool {
	if !p.HasSource() {
		return true
	}
	if _, delegates := s.Delegated(p); delegates {
		return false
	}

	// Everything Delegated refuses is already gone. What remains is either a
	// project with no flat config at all — Apply writes one, so nothing is
	// satisfied yet — or a config jsconfig recognises with well-formed
	// markers, which converges by byte comparison.
	path := eslintFlatConfig(p.Source)
	if path == "" {
		return false
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	candidate, err := spliceEslintConfig(p, path, raw)
	if err != nil {
		return false
	}
	return bytes.Equal(candidate, raw)
}

func (eslintExtendsStep) Describe(p project.Project) string {
	// moduleSpecifier, not ownedFrom's raw path: what this describes is the
	// line dharness is about to write into JavaScript, and the two differ —
	// a reader who copies the path form gets a bare specifier Node cannot
	// resolve.
	target := moduleSpecifier(ownedFrom(p, p.Source, ownedEslintName(projectEslintModule(p))))
	return fmt.Sprintf(
		"ESLint's flat config has no `extends` of its own, so the layer arrives by\nimport and spread instead. A new %s is written:\n\n    import dharnessLayer from %q;\n\n    export default [...dharnessLayer({ plugin: dharnessPlugin })];",
		eslintConfig, target)
}

// Delegated answers the full eslint-config-splice refusal matrix: a
// TypeScript config, a legacy-only project, an unreadable file, a malformed
// marker pair, or a shape jsconfig.Analyze does not recognise all delegate
// (ok == true). No config at all, or an existing flat config jsconfig
// recognises with well-formed markers, does not (ok == false) — the
// write-if-absent and future splice/replace paths both start from there.
func (eslintExtendsStep) Delegated(p project.Project) (string, bool) {
	if !p.HasSource() {
		return "", false
	}

	if path := eslintTypeScriptConfig(p.Source); path != "" {
		return fmt.Sprintf(
			"%s is TypeScript, and dharness does not parse it — a second grammar plus\ntyped helpers whose semantics belong to the project, the same reasoning §03's\namendment already applies to Stryker configs.",
			filepath.Base(path)), true
	}

	if path := eslintFlatConfig(p.Source); path != "" {
		return delegateFlatEslintConfig(path)
	}

	if path := eslintLegacyConfig(p.Source); path != "" {
		return fmt.Sprintf(
			"%s is this project's only ESLint configuration, and dharness does not\ncompose a flat config on top of a legacy one — a legacy ESLint config,\nmatching legacyLintConfigStep's own reasoning for the analogous\ndoctor-config case. Migrating to flat config is the project's decision.",
			filepath.Base(path)), true
	}

	return "", false
}

// delegateFlatEslintConfig reads an existing flat config and answers every
// remaining refusal-matrix cell over it: an unreadable file, a malformed
// marker pair, or whatever jsconfig.Analyze itself refuses — an
// unrecognised call expression, an ERROR node, a non-array export.
// Anything jsconfig accepts, with well-formed markers, is not delegated.
func delegateFlatEslintConfig(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s could not be read: %s", filepath.Base(path), err), true
	}

	if why, malformed := malformedEslintMarkerPair(string(raw)); malformed {
		return why, true
	}

	if _, why, ok := jsconfig.Analyze(raw); !ok {
		return why, true
	}

	return "", false
}

// malformedEslintMarkerPair checks both marker pairs eslintExtendsStep owns
// and reports the first malformed one found, or ("", false) when neither
// is.
func malformedEslintMarkerPair(raw string) (why string, malformed bool) {
	if _, _, state := markerRegion(raw, eslintImportBegin, eslintImportEnd); state == markersMalformed {
		return "the dharness:eslint-import marker pair is malformed — a begin with no\nmatching end, an end with no matching begin, or more than one of either —\nand dharness refuses to guess at a half-written region.", true
	}
	if _, _, state := markerRegion(raw, eslintLayerBegin, eslintLayerEnd); state == markersMalformed {
		return "the dharness:eslint-layer marker pair is malformed — a begin with no\nmatching end, an end with no matching begin, or more than one of either —\nand dharness refuses to guess at a half-written region.", true
	}
	return "", false
}

// Apply writes the config that has none yet (unchanged from slice 3a), or
// splices an existing one: insert when neither marked region exists,
// replace in place when both already do. The candidate is verified — no
// ERROR node, exactly one well-formed region of each marker kind — before
// anything is written, so an unparseable result never reaches disk at all
// (design decision 6); Writer.Undo remains the backstop for steps that ran
// earlier in this same applySteps call, not for this file.
func (eslintExtendsStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	path := eslintFlatConfig(p.Source)
	if path == "" {
		return Facts{}, wireEslintExtends(p, w)
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return Facts{}, fmt.Errorf("read %s before splicing: %w", filepath.Base(path), err)
	}

	candidate, err := spliceEslintConfig(p, path, src)
	if err != nil {
		return Facts{}, err
	}
	if err := verifyEslintCandidate(candidate); err != nil {
		return Facts{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return Facts{}, w.Write(path, candidate)
}

// --------------------------------------------------- boundaries owner
//
// fallow's `extends` replaces a key rather than merging it. Measured against
// fallow 3.14.0: a parent declaring a key is honoured until the child
// declares its own, and from then on the parent's value is discarded whole —
// no error, no warning, and the `extends` line still reads as correct.
//
// That makes it the one way any key dharness (directly, for `boundaries`, or
// through a matched preset, for whatever it contributes) writes can stop
// taking effect while everything else still looks wired, which is why it
// gets a step rather than a line in another step's Describe. `boundaries` is
// a fixed member of the set checked regardless of which preset matched —
// ownedFilesStep writes the architecture block for every project, not only
// preset-matched ones — and every key a matched preset contributes joins it
// (framework-presets design decision 8's second guard, generalised by
// decision 5).

type boundariesOwnerStep struct{}

func (boundariesOwnerStep) ID() string {
	return "resolve the keys this project and dharness both declare"
}

// collidingKeys is this step's whole rule: matches' contributed keys plus
// the fixed "boundaries" member, intersected with what the project's own
// fallow config declares. Split out from the step's methods for the same
// reason resolve is split out of Resolve (framework-presets design decision
// 3): the real registry contributes nothing beyond "boundaries" until slice
// 5 registers a framework preset, so the intersection rule is tested against
// stub matches instead.
func collidingKeys(source string, matches []preset.Match) []string {
	candidates := append([]string{"boundaries"}, preset.Keys(matches)...)
	return declaredKeys(fallowConfigPath(source), candidates)
}

// boundaryCollision computes the collision set for p, guarded the same way
// Satisfied always was: with no JS project there is no config to read, and
// p.Source empty would resolve fallowConfigPath relative to the working
// directory instead of nowhere — the unsafe read
// TestBoundariesOwnerStepIsSatisfiedWithoutASource already guards against.
//
// A config declaredKeys cannot read — fallow.toml, whose keys are bare — is
// not this step's business. It is a blind spot, and UncheckableConfigNote
// reports it beside the plan rather than inside it: see that function for
// why a blind spot is not a step.
func boundaryCollision(p project.Project) (colliding []string, matches []preset.Match) {
	matches = preset.Resolve(p)
	if !p.HasSource() {
		return nil, matches
	}
	return collidingKeys(p.Source, matches), matches
}

func (boundariesOwnerStep) Satisfied(p project.Project) bool {
	colliding, _ := boundaryCollision(p)
	return len(colliding) == 0
}

// resolvedConfig asks fallow which value it actually runs for each key, or
// reports that it could not be asked. Two outcomes and no third: a resolved
// map, or nothing. Nothing is never an error this run reports — a missing
// measurement is not a failed sync (§20).
//
// The probe runs first and its own exit code decides everything: --path
// exits 3 on a project with no fallow config at all, and --format json
// never exits non-zero even then — it prints defaults, so it is useless for
// detecting absence (measured against the reference project, design.md
// Decision 5). Any other exit from the probe collapses to the same absence,
// and the resolve call never runs at all — the short circuit is on any
// non-nil error, not only on code 3.
//
// Resolution is local-only and this is deliberate, not an oversight:
// p.LocalBinary(tool.Fallow) is the only path in, with no remote-executor
// fallback, per the resolved question round and the network rule
// internal/tool/tool.go:101-103 already records for every routine path.
func resolvedConfig(p project.Project) (map[string]json.RawMessage, bool) {
	localBinary := p.LocalBinary(tool.Fallow)
	if localBinary == "" {
		return nil, false
	}

	probe := tool.Installed(tool.Fallow, localBinary, p.Source, tool.FallowConfigPath()...)
	if err := runner.Run(probe, io.Discard, io.Discard); err != nil {
		return nil, false
	}

	var stdout bytes.Buffer
	resolve := tool.Installed(tool.Fallow, localBinary, p.Source, tool.FallowConfigJSON()...)
	if err := runner.Run(resolve, &stdout, io.Discard); err != nil {
		return nil, false
	}

	var resolved map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &resolved); err != nil {
		return nil, false
	}
	return resolved, true
}

// Collisions computes each colliding key once: the value both the report's
// collision block and boundariesOwnerStep.Delegated are rendered from.
// Exported as a package function rather than reached through the Step
// interface, matching UncheckableConfigNote/UncertainPresetNote/
// eslintResidue at their own call site (collectNotes, notes.go) — so no
// requirement about Delegated's (why string, ok bool) contract is reopened
// and nothing type-asserts on a step.
//
// Slice 3 computed this value; describeBoundaries/delegateBoundaries are
// wired to render from it, replacing their own walk, in this slice
// (design.md Decision 4's explicit ordering note — landing the two apart
// would leave a window where a key renders twice).
func Collisions(p project.Project) []report.Collision {
	colliding, matches := boundaryCollision(p)
	if len(colliding) == 0 {
		return nil
	}

	resolved, ok := resolvedConfig(p)
	path := fallowConfigPath(p.Source)
	// Theirs.Path is rendered directly in the human collision block (gap 1),
	// so it is stated the same root-relative way Ours.Path and
	// Report.Source already are — fallowConfigPath itself returns an
	// absolute filesystem path, which is unreadable there (measured against
	// a real project during verification: "project
	// C:\Users\...\frontend\.fallowrc.json"). declaredAt still reads from
	// the real absolute path; only the displayed value is relativised.
	displayPath := path
	if rel, err := filepath.Rel(p.Root, path); err == nil {
		displayPath = rel
	}

	collisions := make([]report.Collision, 0, len(colliding))
	for _, key := range colliding {
		collision := report.Collision{
			ID:   "sync:collision/" + key,
			Key:  key,
			Ours: ourDeclared(p, key, matches),
			Theirs: report.Declared{
				Path: filepath.ToSlash(displayPath),
				Line: declaredAt(path, key),
			},
			// Resolutions is always the same pair, whether or not effective
			// was ever measured: deleting the project's own declaration or
			// moving it into the file dharness owns are the only two ways
			// any collision on a key dharness also owns can be resolved —
			// unlike effective, this fact needs no measurement to state
			// (gap 11: the field existed on the model since slice 1 and was
			// never populated here).
			Resolutions: collisionResolutions,
		}

		if ok {
			if raw, present := resolved[key]; present {
				theirsValue := json.RawMessage(raw)
				collision.Theirs.Value = &theirsValue

				effective := "theirs"
				if bytes.Equal(bytes.TrimSpace(raw), bytes.TrimSpace(*collision.Ours.Value)) {
					effective = "ours"
				}
				collision.Effective = &effective
			}
		}

		collisions = append(collisions, collision)
	}
	return collisions
}

// collisionResolutions are the two ways any config collision on a key
// dharness also owns can be resolved. There is no third: both sides declare
// the same key, and fallow's `extends` can only ever honour one of them.
var collisionResolutions = []string{"delete-theirs", "move-into-ours"}

// ourDeclared reports dharness's own side of a collision: the value it
// wrote, or is about to write, into the file it owns — reported whole, per
// the "colliding value is reported whole or not at all" requirement — and
// where that file is: the owned fallow.jsonc always, and the line inside it
// that declares key when the file already exists on disk (gap 10 — Path was
// left empty, so the human view's "dharness: <value>" had nowhere to point
// a reader). By the time boundariesOwnerStep is evaluated inside setup.Run,
// ownedFilesStep has already applied earlier in Plan() order and written
// this same file, so the line is measured from the real file, not guessed
// at; declaredAt's own documented sentinel (0, omitted by Declared.Line's
// json tag) covers the case where it does not exist yet, matching Theirs'
// own already-established absent-line behaviour.
//
// A matched preset's composed fact already marshals to JSON through
// ownedValue's own json.Marshal; boundaries' own case has no fixed value
// yet — it is agent-authored — so ownedValue's prose is carried as a JSON
// string instead of guessed at as structured data.
//
// json.Marshal of a Go string cannot fail — it has no channel, function or
// cycle to reject — so there is no error branch here to check for one.
func ourDeclared(p project.Project, key string, matches []preset.Match) report.Declared {
	value := ownedValue(key, matches)
	raw := json.RawMessage(value)
	if !json.Valid(raw) {
		encoded, _ := json.Marshal(value)
		raw = json.RawMessage(encoded)
	}

	ownedPath := filepath.Join(p.Root, project.Dir, ownedFallow)
	return report.Declared{
		Path:  filepath.ToSlash(filepath.Join(project.Dir, ownedFallow)),
		Line:  declaredAt(ownedPath, key),
		Value: &raw,
	}
}

// boundariesFallbackDescribe/Why are the single-key text this step always
// produced before this slice widened it. They are the fallback the empty
// intersection reaches, not a stale duplicate: the golden fixtures render
// every step's report unconditionally (framework-presets design decision 7)
// for a generic project, where the intersection is always empty, and that
// report has always read this way. Changing it here would fail a
// byte-identity requirement no real collision ever reaches in practice,
// since Pending only calls Describe/Delegated for a step Satisfied has
// already reported false for.
const boundariesFallbackDescribe = "Move the zones and rules from %s into %s, or delete the block dharness\nowns and keep the project's. Either is a valid answer; having both is not,\nbecause only one of them runs and the file gives no sign of which."
const boundariesFallbackWhy = "%s declares its own `boundaries`, and fallow's `extends` replaces that key\nrather than merging it — the project's block replaces the one dharness owns\nentirely, without an error. Only one architecture is being enforced, and the\nconfiguration does not say which."

func (boundariesOwnerStep) Describe(p project.Project) string {
	colliding, _ := boundaryCollision(p)
	return describeBoundaries(p, colliding)
}

// describeBoundaries and delegateBoundaries build the step's report from an
// already-computed collision set. The empty-intersection fallback stays a
// fixed pair of constants (Decision 6, guarded by
// TestBoundariesFallbackConstantsStayByteIdentical); the non-empty case
// renders from Collisions(p) — the same computed value the report carries
// on StepResult.Collisions (design.md Decision 4) — rather than walking the
// colliding keys a second time.
func describeBoundaries(p project.Project, colliding []string) string {
	if len(colliding) == 0 {
		return fmt.Sprintf(boundariesFallbackDescribe, fallowConfig, filepath.ToSlash(filepath.Join(project.Dir, ownedFallow)))
	}
	return renderCollisions(Collisions(p))
}

// Delegated always returns ok == true where the step is unsatisfied: two
// values for the same key cannot be merged by a rule. Which one survives is
// a decision about intent, and dharness does not hold it. It names both
// values per colliding key, not only the key, so the agent does not have to
// open two files before deciding.
func (boundariesOwnerStep) Delegated(p project.Project) (string, bool) {
	colliding, _ := boundaryCollision(p)
	return delegateBoundaries(p, colliding), true
}

func delegateBoundaries(p project.Project, colliding []string) string {
	if len(colliding) == 0 {
		return fmt.Sprintf(boundariesFallbackWhy, fallowConfig)
	}
	return renderCollisions(Collisions(p))
}

// renderCollisions is the only place a Collision becomes prose (design.md
// Decision 4). Delegated calls it, through delegateBoundaries; the report's
// own WriteHuman/WriteJSON render StepResult.Collisions directly and never
// call it, so one colliding key has exactly one prose form and one
// structured form — never two independently-walked renderers that could
// drift apart (defects 6, 7).
func renderCollisions(cs []report.Collision) string {
	keys := make([]string, len(cs))
	for i, c := range cs {
		keys[i] = c.Key
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s declares its own value for %s, and fallow's `extends` replaces each of\nthose keys rather than merging it — the project's value replaces dharness's\nentirely, without an error. Only one value per key is being enforced, and\nthe configuration does not say which:\n", fallowConfig, quotedKeys(keys))
	for _, c := range cs {
		theirs := declaredValueUnknown
		if c.Theirs.Value != nil {
			theirs = string(*c.Theirs.Value)
		}
		fmt.Fprintf(&b, "\n`%s` — dharness: %s; %s: %s", c.Key, string(*c.Ours.Value), fallowConfig, theirs)
	}
	return b.String()
}

// UncheckableConfigNote reports a fallow config dharness cannot read
// textually, and returns "" when there is nothing to say.
//
// It is a note rather than a step on purpose. fallow.toml's keys are bare, so
// the quoted-key test declaredKeys uses cannot answer for it, and parsing
// TOML exactly is not something this product does. An unsatisfied step would
// put an entry in the plan the project can never clear: the agent can read
// the file by hand, but no state it reaches makes dharness stop asking, and
// §15 says a step disappears once satisfied. A step with no completion state
// is not pending work.
//
// The other direction was worse. Answering "nothing collides" for a file
// dharness never read is the silent no-op this whole change exists to end, so
// the report says the answer is unknown rather than clear.
func UncheckableConfigNote(p project.Project) string {
	if !p.HasSource() || fallowConfigPath(p.Source) != "" || !p.HasFallowConfig() {
		return ""
	}
	return fmt.Sprintf(
		"fallow.toml is this project's only fallow config, and dharness reads these\nfiles textually rather than parsing them — TOML's keys are bare, so the test\nit uses for .fallowrc.json and .fallowrc.jsonc cannot answer for it. Whether\nit declares a key %s also writes is unknown, not clear. Check it by hand.",
		filepath.ToSlash(filepath.Join(project.Dir, ownedFallow)))
}

// UncertainPresetNote reports what a matched preset could not read about
// this project, or "" when every match read everything it needed. Beside
// the plan, not inside it — the same reason UncheckableConfigNote is: a
// blind spot with no resolution the project can reach is a note, never an
// unsatisfiable step (§15). A preset that could not read its own
// configuration still matched and still contributed its documented default
// (Match.Uncertain is set beside a real Manifest, not instead of one), so
// this note says what it guessed from, not that something is broken.
func UncertainPresetNote(p project.Project) string {
	return uncertainNotes(preset.Resolve(p))
}

// uncertainNotes is UncertainPresetNote over an explicit match list, split
// out for the same reason collidingKeys is: no real preset carried a
// non-empty Uncertain until wails registered, so the rendering rule is
// tested against stub matches too.
func uncertainNotes(matches []preset.Match) string {
	var notes []string
	for _, match := range matches {
		if match.Uncertain != "" {
			notes = append(notes, fmt.Sprintf("%s: %s", match.ID, match.Uncertain))
		}
	}
	return strings.Join(notes, "\n\n")
}

// eslintResidue reports the six dharness/* severities and the RulesPackage
// plugin declaration a repository adopted before this change still carries
// in doctor.config.json, left behind by the mechanism this version retires
// (design decision 8) — entries found, the path they were found in, and why
// they are inert, or (nil, "", "") when there is nothing to report, which is
// every project adopted under this version and any older one hand-cleaned.
//
// It is a note beside the plan, not a step, for UncheckableConfigNote's exact
// reason: dharness cannot tell its own earlier write apart from a value the
// project set into the same file afterwards (§05), so there is no state the
// project can reach that clears it — and a step with no completion state is
// not pending work. Unlike UncheckableConfigNote's blind spot, this is not an
// unknown: dharness can read the file fine and knows exactly what it holds,
// so it reports the found entries as structured data (report.Note.Entries)
// rather than folding them into the reason's prose a second time.
func eslintResidue(p project.Project) (entries []string, path string, reason string) {
	if !p.HasSource() {
		return nil, "", ""
	}

	path = filepath.Join(p.Source, "doctor.config.json")
	candidates := append([]string{RulesPackage}, RuleIDs()...)
	found := declaredKeys(path, candidates)
	if len(found) == 0 {
		return nil, "", ""
	}

	reason = "dharness does not edit or delete them — it cannot tell its own earlier\nwrite apart from a value the project set into the same file afterwards\n(§05) — and they are inert now: the gate's react-doctor invocation runs\nwith `--staged`, and a plugin's rules do not fire under that flag\n(measured against react-doctor 0.5.7)."
	return found, path, reason
}

// ownedValue names what dharness itself would write for key: the
// architecture block the agent writes for "boundaries" — no preset may ever
// contribute that key (framework-presets design decision 2's guard) — or a
// matched preset's composed value otherwise.
func ownedValue(key string, matches []preset.Match) string {
	if key == "boundaries" {
		return "the architecture block the agent writes"
	}
	for _, fact := range composeFacts(matches) {
		if fact.key == key {
			if raw, err := json.Marshal(fact.value); err == nil {
				return string(raw)
			}
		}
	}
	return "its preset default"
}

// declaredValueUnknown is renderCollisions's fallback for a colliding key
// whose project-declared value fallow could not resolve (no local binary,
// a failed probe, or the key absent from what --format json returned) — the
// honest "could not be shown" answer the "reported whole or not at all"
// requirement asks for, never the truncated textual fragment
// ("\"duplicates\": {") declaredValue used to show before design.md
// Decision 5 removed it from the collision path entirely (defect 8).
const declaredValueUnknown = "a value of its own"

// quotedKeys renders a list of keys the way this file already names fallow
// keys elsewhere — backticked, not Go-quoted.
func quotedKeys(keys []string) string {
	backticked := make([]string, len(keys))
	for i, key := range keys {
		backticked[i] = "`" + key + "`"
	}
	return strings.Join(backticked, ", ")
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion, matching architectureStep.
func (boundariesOwnerStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	return Facts{}, fmt.Errorf("%s is delegated and must not be applied", boundariesOwnerStep{}.ID())
}

// ------------------------------------------------ legacy lint config
//
// dharness does not run ESLint and does not want to. This step exists because
// react-doctor does something dharness's gate then depends on: it adopts the
// project's `.eslintrc.json` and runs the rules declared there.
//
// Measured against react-doctor 0.5.7. A valid file reports its rules as
// `eslint/no-console` and exits 1. The same file with a trailing comma
// reports nothing, exits 0, and names the file nowhere — not even under
// --verbose. The gate goes green having quietly stopped enforcing rules it
// was enforcing a moment earlier.
//
// Only this one format is adopted. `.eslintrc.cjs`, `.eslintrc.yml`,
// `package.json`'s eslintConfig key and flat config were each measured and
// ignored, so this is one file rather than a family — and a project on flat
// config, the ESLint 9 default, has nothing here however broken its config.

type legacyLintConfigStep struct{}

func (legacyLintConfigStep) ID() string {
	return "fix the lint config react-doctor silently drops"
}

func (legacyLintConfigStep) Satisfied(p project.Project) bool {
	if !p.HasSource() {
		return true
	}
	raw, err := os.ReadFile(filepath.Join(p.Source, legacyLintConfig))
	if err != nil {
		return true
	}
	return json.Valid(raw)
}

func (legacyLintConfigStep) Describe(project.Project) string {
	return fmt.Sprintf(
		"Make %s parse, or delete it. Either answer is fine; a file that is present\nand unreadable is not, because the gate cannot tell you which rules it stopped\nrunning.",
		legacyLintConfig)
}

// Delegated always returns ok == true: the project's lint rules are the
// project's, and repairing broken JSON means deciding what it was meant to
// say.
func (legacyLintConfigStep) Delegated(project.Project) (string, bool) {
	return fmt.Sprintf(
		"%s does not parse. react-doctor adopts this file and runs the rules in it,\nso a broken one drops them all — measured: the same rules that failed the gate\nwith exit 1 report nothing and it exits 0, naming the file nowhere, not even\nunder --verbose. Repairing it means knowing what it was meant to say.",
		legacyLintConfig), true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion, matching architectureStep.
func (legacyLintConfigStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	return Facts{}, fmt.Errorf("%s is delegated and must not be applied", legacyLintConfigStep{}.ID())
}

// ----------------------------------------------------------------- mcp

type mcpStep struct{}

func (mcpStep) ID() string { return "give the agent fallow's own tools" }

func (mcpStep) Satisfied(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, mcpConfig))
	if err != nil {
		return false
	}
	var config mcpConfigFile
	if json.Unmarshal(raw, &config) != nil {
		return false
	}
	_, registered := config.Servers["fallow"]
	return registered
}

func (mcpStep) Describe(project.Project) string {
	return "fallow ships an MCP server with the analysis an agent would otherwise ask a\nwrapper to invent: boundaries, traces, impact, health."
}

// Delegated is always false: the MCP entry dharness writes has no case where
// it belongs to the project instead.
func (mcpStep) Delegated(project.Project) (string, bool) { return "", false }

func (mcpStep) Apply(p project.Project, w *Writer, _ io.Writer) (Facts, error) {
	path := filepath.Join(p.Root, mcpConfig)

	config := mcpConfigFile{Servers: map[string]mcpServer{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return Facts{}, fmt.Errorf("%s is not readable as JSON, so it cannot be merged: %w", mcpConfig, err)
		}
		if config.Servers == nil {
			config.Servers = map[string]mcpServer{}
		}
	}

	// The binary ships with fallow, so the remote executor names that package
	// explicitly rather than asking the registry for a package named fallow-mcp.
	binary := tool.RemotePackageBinary(p.PackageManager, tool.LatestSpec(tool.Fallow), "fallow-mcp", p.Source)
	config.Servers["fallow"] = mcpServer{
		Command: binary.Name,
		Args:    binary.Args,
	}
	return Facts{}, w.WriteJSON(path, config)
}

type mcpConfigFile struct {
	Servers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// --------------------------------------------------------- hook install

type hookInstallStep struct{}

func (hookInstallStep) ID() string { return "wire the gate into git" }

func (hookInstallStep) Satisfied(p project.Project) bool {
	switch hookManager(p) {
	case managerLefthook:
		return gateInstalled(p)
	case managerHusky:
		return huskyWired(p)
	default:
		// Nothing answers, so there is nothing dharness can satisfy on its
		// own. This is an open decision, not a default dharness gets to
		// pick — see Delegated.
		return false
	}
}

func (hookInstallStep) Describe(p project.Project) string {
	switch hookManager(p) {
	case managerHusky:
		return fmt.Sprintf("husky keeps a shell script, so the gate is one appended line in %s.", huskyHook)
	case managerLefthook:
		return "lefthook writes the git hook itself; without running it the configuration\nexists and nothing calls it."
	default:
		return fmt.Sprintf(
			"Choose a hook manager and wire it to run the gate:\n\n    %s\n\nlefthook writes its own git hook once configured and installed; husky keeps\na shell script that needs one appended line. Either way, the step is\nsatisfied once %q runs on commit.",
			gateCommand, gateCommand)
	}
}

// Delegated per manager: lefthook and husky both answer, so dharness installs
// the gate itself. No manager answering is Decision 5's open decision — it is
// not dharness's default to pick.
//
// The lefthook branch carries one more condition, and it is a dependency on
// step 5 rather than a second rule: `lefthook install` reads the project's
// lefthook.yml, so a config that does not carry the reference to the file
// dharness owns installs nothing whatever this step does. When step 5 handed
// that merge to the project, this step is waiting on the same decision and
// says so, instead of running a command that cannot work and failing the run
// over an answer nobody has given yet.
func (hookInstallStep) Delegated(p project.Project) (string, bool) {
	switch hookManager(p) {
	case managerHusky:
		return "", false
	case managerLefthook:
		if _, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
			return fmt.Sprintf(
				"%s has to gain the reference first — see the step above. lefthook installs\nthe hooks its own config declares, so until that merge happens there is no\npre-commit job for it to install.",
				lefthookConfig), true
		}
		return "", false
	default:
		return "nothing answers: there is no lefthook config, no .husky/ and no lefthook\nbinary. Choosing a hook manager is a decision this project has not made, and\nnot a default dharness gets to pick." + blockedPostinstallNote(p), true
	}
}

// blockedPostinstallNote warns a bun project about the step between
// installing lefthook and having a lefthook that runs.
//
// `bun add -d lefthook` succeeds and prints "Blocked 1 postinstall", because
// bun does not run lifecycle scripts by default — and lefthook's postinstall
// is what downloads the platform binary. So the command this delegation asks
// for leaves an unusable binary behind, with the reason two lines up in
// output that has already scrolled past.
//
// Not dharness's defect, and saying so is not dharness doing bun's job
// either: the note sits directly in the path of a step dharness chose to
// delegate, and dharness has already detected the package manager. Only bun
// gets it — the other three run the postinstall, and a sentence about a
// problem they do not have is noise in a block meant to be read.
func blockedPostinstallNote(p project.Project) string {
	if p.PackageManager != "bun" {
		return ""
	}
	return "\n\nOn bun, lifecycle scripts are blocked by default, so `bun add -d lefthook`\ninstalls the package without downloading its binary. `bun pm trust lefthook`\nruns the postinstall that does."
}

func (hookInstallStep) Apply(p project.Project, w *Writer, out io.Writer) (Facts, error) {
	if hookManager(p) == managerHusky {
		return Facts{}, appendHuskyGate(p, w)
	}

	path := p.LocalBinary("lefthook")
	command := tool.RemoteLatest(p.PackageManager, "lefthook", p.Root, "install")
	if path != "" {
		command = tool.Installed("lefthook", path, p.Root, "install")
	}
	return Facts{}, runner.Run(command, out, out)
}

// Verify is the whole of F1: this step's Satisfied already answers the exact
// question Apply cannot — will git run the gate — so the postcondition is
// Satisfied itself, asked once more after the subprocess claims success.
//
// That equivalence is not general. installStep's Satisfied deliberately
// answers a different question ("is there a JS project here at all") and is
// false right after a successful install, which is why run asks this through
// an interface a step opts into rather than re-checking Satisfied for
// everyone.
//
// The error names the file that would have proved it, because the fix is
// almost never in dharness: a lefthook.yml whose pre-commit job is commented
// out, or one carrying the project's own jobs and not the gate, produces
// exactly this — `lefthook install` exits 0 and writes no pre-commit hook.
func (s hookInstallStep) Verify(p project.Project) error {
	if s.Satisfied(p) {
		return nil
	}

	if hookManager(p) == managerHusky {
		return fmt.Errorf(
			"the hook manager reported success, but %s does not carry %q; nothing runs the gate on commit",
			huskyHook, gateCommand)
	}

	hooks, err := project.HooksDir(p.Root)
	if err != nil {
		return fmt.Errorf("the hook manager reported success, but git cannot name this repository's hooks directory: %w", err)
	}
	return fmt.Errorf(
		"the hook manager reported success, but no pre-commit hook was installed in %s; check that %s declares a pre-commit job — lefthook exits 0 and installs nothing when it has none",
		filepath.ToSlash(hooks), lefthookConfig)
}

// --------------------------------------------------------- agent skill

type agentSkillStep struct{}

func (agentSkillStep) ID() string { return "install react-doctor's agent skill" }

func (agentSkillStep) Satisfied(p project.Project) bool {
	// The installer writes into whichever agents it detects, so the honest
	// check is whether any of the places it would write already carries it.
	for _, candidate := range skillLocations {
		if _, err := os.Stat(filepath.Join(p.Root, candidate)); err == nil {
			return true
		}
	}
	return false
}

func (agentSkillStep) Describe(p project.Project) string {
	return fmt.Sprintf("    %s %s install\n\nChoose the skill and decline the rest.", tool.RemoteExec(p.PackageManager), tool.LatestSpec(tool.ReactDoctor))
}

// Delegated always returns ok == true: no non-interactive install exists that
// installs only the skill, so this step is always the agent's to run.
func (agentSkillStep) Delegated(project.Project) (string, bool) {
	return "its only non-interactive form installs five things: skills for every agent it\ndetects, a package script, a git hook that competes with this gate, and a CI\nworkflow. There is no flag to ask for the skill alone.", true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion — see TestAgentSkillApplyIsUnreachable.
func (agentSkillStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	return Facts{}, fmt.Errorf("%s is delegated and must not be applied", agentSkillStep{}.ID())
}

// -------------------------------------------------------- architecture

type architectureStep struct{}

func (architectureStep) ID() string { return "decide this project's architecture" }

// Satisfied follows the extendsWired precedent: a substring check on the
// literal text, not a JSONC parse. The product is stdlib-only, and "does the
// file declare a boundaries block" does not need a parser to answer.
func (architectureStep) Satisfied(p project.Project) bool {
	return len(declaredKeys(filepath.Join(p.Root, project.Dir, ownedFallow), []string{"boundaries"})) > 0
}

func (architectureStep) Describe(p project.Project) string {
	return ArchitecturePrompt(p)
}

// Delegated always returns ok == true: this is Intención, and no detection
// tells dharness what a project's architecture is meant to be.
func (architectureStep) Delegated(project.Project) (string, bool) {
	return "architecture boundaries say what the code is meant to be, and no tool can\nread intent off a tree. Do the analysis and write the result — nothing else\nneeds to change.", true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion, matching agentSkillStep.
func (architectureStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	return Facts{}, fmt.Errorf("%s is delegated and must not be applied", architectureStep{}.ID())
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}
