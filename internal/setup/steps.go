package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
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
		tool.InstallCommand(p.PackageManager), strings.Join(integrationPackages(), " "))
}

// Delegated is always false: there is no repository state that hands
// installing a package to the agent instead of dharness.
func (installStep) Delegated(project.Project) (string, bool) { return "", false }

func (installStep) Apply(p project.Project, w *Writer) error {
	packages := integrationPackages()

	for _, path := range p.PackageStateFiles() {
		if err := w.remember(path); err != nil {
			return fmt.Errorf("snapshot package state before install: %w", err)
		}
	}

	installErr := runner.Run(tool.InstallPackages(p.PackageManager, p.Source, packages), os.Stdout, os.Stderr)
	w.compensate(func() error {
		if err := runner.Run(tool.RemovePackages(p.PackageManager, p.Source, packages), os.Stdout, os.Stderr); err != nil {
			return fmt.Errorf("remove integration packages added by this run: %w", err)
		}
		return nil
	})
	return installErr
}

// integrationPackages lists the packages dharness adds to a project, as
// opposed to the CLIs it invokes without installing.
func integrationPackages() []string {
	return dedupe([]string{RulesPackage})
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
	eslint, _ := os.ReadFile(filepath.Join(p.Root, project.Dir, ownedEslint))
	if string(eslint) != ownedEslintConfig(p, preset.Layers(matches)) {
		return false
	}

	// The allow list is repaired rather than written once (design decision
	// 2): a repository adopted before this change predates the entry, and
	// hand-removing it must bring the step back the same way any other
	// derived state does (§15). Same reasoning as above: a missing
	// .gitignore reads back as empty bytes, which never contains the
	// non-empty entry string either.
	ignore, _ := os.ReadFile(filepath.Join(p.Root, project.Dir, ".gitignore"))
	return strings.Contains(string(ignore), "!"+ownedEslint)
}

func (ownedFilesStep) Describe(project.Project) string {
	return fmt.Sprintf("The gate, the architecture and the rule thresholds live in %s/, which is\ncommitted. The project's own files only ever gain one line pointing at them.",
		project.Dir)
}

// Delegated is always false: the files dharness owns are always dharness's to
// write.
func (ownedFilesStep) Delegated(project.Project) (string, bool) { return "", false }

func (ownedFilesStep) Apply(p project.Project, w *Writer) error {
	// EnsureDir also writes the ignore rules, so a transient file appearing
	// later is never the first thing to create them.
	if _, err := p.EnsureDir(""); err != nil {
		return err
	}

	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedLefthook), []byte(gateConfig(p))); err != nil {
		return err
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
		return err
	}

	eslintPath := filepath.Join(p.Root, project.Dir, ownedEslint)
	if err := w.Write(eslintPath, []byte(ownedEslintConfig(p, preset.Layers(matches)))); err != nil {
		return err
	}
	if err := ensureShared(p, w, ownedEslint); err != nil {
		return err
	}

	return w.WriteJSON(filepath.Join(p.Root, project.Dir, ownedRules), DefaultThresholds())
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

func (fallowExtendsStep) Apply(p project.Project, w *Writer) error {
	return wireFallowExtends(p, w)
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
// lefthook.yml instead of .fallowrc.json.
func (lefthookExtendsStep) Delegated(p project.Project) (string, bool) {
	if hookManager(p) != managerLefthook {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(p.Root, lefthookConfig)); errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return fmt.Sprintf(
		"%s already exists and belongs to the project; adding a key to it is a merge,\nnot a write.",
		lefthookConfig), true
}

func (lefthookExtendsStep) Apply(p project.Project, w *Writer) error {
	return wireLefthookExtends(p, w)
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
	return "resolve the two architectures this project declares"
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
	colliding, matches := boundaryCollision(p)
	return describeBoundaries(p, colliding, matches)
}

// describeBoundaries and delegateBoundaries build the step's report from an
// already-computed collision set and match list, split out for the same
// reason collidingKeys is: the real registry contributes nothing beyond
// "boundaries" until slice 5, so the multi-key rendering rule is tested
// against stub matches instead of the real one.
func describeBoundaries(p project.Project, colliding []string, matches []preset.Match) string {
	if len(colliding) == 0 {
		return fmt.Sprintf(boundariesFallbackDescribe, fallowConfig, filepath.ToSlash(filepath.Join(project.Dir, ownedFallow)))
	}

	var b strings.Builder
	b.WriteString("Move each of these into the file dharness owns, or delete the project's own\nand keep dharness's. Either is a valid answer per key; having both is not,\nbecause fallow's `extends` replaces a key rather than merging it and the\nfile gives no sign of which value runs.\n")
	for _, key := range colliding {
		fmt.Fprintf(&b, "\n`%s` — dharness: %s\n%s declares: %s\n",
			key, ownedValue(key, matches), fallowConfig, declaredValue(fallowConfigPath(p.Source), key))
	}
	return b.String()
}

// Delegated always returns ok == true where the step is unsatisfied: two
// values for the same key cannot be merged by a rule. Which one survives is
// a decision about intent, and dharness does not hold it. It names both
// values per colliding key, not only the key, so the agent does not have to
// open two files before deciding.
func (boundariesOwnerStep) Delegated(p project.Project) (string, bool) {
	colliding, matches := boundaryCollision(p)
	return delegateBoundaries(p, colliding, matches), true
}

func delegateBoundaries(p project.Project, colliding []string, matches []preset.Match) string {
	if len(colliding) == 0 {
		return fmt.Sprintf(boundariesFallbackWhy, fallowConfig)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s declares its own value for %s, and fallow's `extends` replaces each of\nthose keys rather than merging it — the project's value replaces dharness's\nentirely, without an error. Only one value per key is being enforced, and\nthe configuration does not say which:\n", fallowConfig, quotedKeys(colliding))
	for _, key := range colliding {
		fmt.Fprintf(&b, "\n`%s` — dharness: %s; %s: %s",
			key, ownedValue(key, matches), fallowConfig, declaredValue(fallowConfigPath(p.Source), key))
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

// declaredValue is a best-effort display of what the project itself wrote,
// falling back to a generic phrase when declaredLine finds no single line to
// show (a value declaredKeys still detected but that spans lines it does not
// follow).
func declaredValue(path, key string) string {
	if line := declaredLine(path, key); line != "" {
		return line
	}
	return "a value of its own"
}

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
func (boundariesOwnerStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", boundariesOwnerStep{}.ID())
}

// -------------------------------------------------------- doctor config

type doctorConfigStep struct{}

func (doctorConfigStep) ID() string { return "declare the rules react-doctor should run" }

func (doctorConfigStep) Satisfied(p project.Project) bool {
	if !p.HasSource() {
		return true
	}
	raw, err := os.ReadFile(filepath.Join(p.Source, doctorConfig))
	if err != nil {
		return false
	}
	var config doctorConfigFile
	if json.Unmarshal(raw, &config) != nil {
		return false
	}
	for _, plugin := range config.Plugins {
		if plugin == RulesPackage {
			return true
		}
	}
	return false
}

func (doctorConfigStep) Describe(project.Project) string {
	return fmt.Sprintf("react-doctor does not compose, so this is a merge rather than a reference: the\npackage joins `plugins` and its %d rules join `rules`.", len(Rules))
}

// Delegated is always false: the merge doctorConfigStep performs has no case
// where dharness cannot perform it itself.
func (doctorConfigStep) Delegated(project.Project) (string, bool) { return "", false }

func (doctorConfigStep) Apply(p project.Project, w *Writer) error {
	path := filepath.Join(p.Source, doctorConfig)

	config := doctorConfigFile{Rules: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return fmt.Errorf("%s is not readable as JSON, so it cannot be merged: %w", doctorConfig, err)
		}
		if config.Rules == nil {
			config.Rules = map[string]string{}
		}
	}

	config.Plugins = dedupe(append(config.Plugins, RulesPackage))
	for _, id := range RuleIDs() {
		if _, chosen := config.Rules[id]; !chosen {
			config.Rules[id] = DefaultSeverity(p, id)
		}
	}
	return w.WriteJSON(path, config)
}

type doctorConfigFile struct {
	Plugins []string          `json:"plugins,omitempty"`
	Rules   map[string]string `json:"rules,omitempty"`
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
func (legacyLintConfigStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", legacyLintConfigStep{}.ID())
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

func (mcpStep) Apply(p project.Project, w *Writer) error {
	path := filepath.Join(p.Root, mcpConfig)

	config := mcpConfigFile{Servers: map[string]mcpServer{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return fmt.Errorf("%s is not readable as JSON, so it cannot be merged: %w", mcpConfig, err)
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
	return w.WriteJSON(path, config)
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
func (hookInstallStep) Delegated(p project.Project) (string, bool) {
	switch hookManager(p) {
	case managerLefthook, managerHusky:
		return "", false
	default:
		return "nothing answers: there is no lefthook config, no .husky/ and no lefthook\nbinary. Choosing a hook manager is a decision this project has not made, and\nnot a default dharness gets to pick.", true
	}
}

func (hookInstallStep) Apply(p project.Project, w *Writer) error {
	if hookManager(p) == managerHusky {
		return appendHuskyGate(p, w)
	}

	path := p.LocalBinary("lefthook")
	command := tool.RemoteLatest(p.PackageManager, "lefthook", p.Root, "install")
	if path != "" {
		command = tool.Installed("lefthook", path, p.Root, "install")
	}
	return runner.Run(command, os.Stdout, os.Stderr)
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
func (agentSkillStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", agentSkillStep{}.ID())
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
func (architectureStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", architectureStep{}.ID())
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
