package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/jsconfig"
	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
)

// The files dharness owns, named after the tool whose format they hold.
const (
	ownedLefthook = "lefthook.yml"
	ownedFallow   = "fallow.jsonc"
	ownedRules    = "rules.json"

	// ownedEslintLegacyName is the single name every dharness before this
	// one wrote, whatever dialect it put inside. It is kept so a project
	// adopted then can have the orphan cleaned up; nothing writes it.
	ownedEslintLegacyName = "eslint.config.js"
)

// ownedEslintName spells the file dharness owns the way Node reads it.
//
// The extension is not decoration here. A .js file is whatever the nearest
// package.json says it is, and a Next.js package.json says nothing — so ESM
// inside one earns MODULE_TYPELESS_PACKAGE_JSON from Node on every ESLint
// run, which for an adopted project is every gated commit, and Node's own
// suggested remedy (add "type": "module") is a far bigger change to the
// project than the warning warrants. The reverse case is worse than noisy:
// module.exports in a .js file under a package.json that does declare
// "type": "module" is a SyntaxError.
//
// .mjs and .cjs answer for themselves, so neither case can arise. The
// project's own config imports the file by path either way.
func ownedEslintName(module jsconfig.Module) string {
	if module == jsconfig.CommonJS {
		return "eslint.config.cjs"
	}
	return "eslint.config.mjs"
}

// The files that belong to the project and gain at most one line.
const (
	lefthookConfig = "lefthook.yml"
	fallowConfig   = ".fallowrc.json"
	eslintConfig   = "eslint.config.js"
	mcpConfig      = ".mcp.json"
	huskyHook      = ".husky/pre-commit"

	// legacyLintConfig is the one lint config react-doctor adopts, and so the
	// only one whose breakage reaches dharness's gate. Flat config, the ESLint
	// 9 default, is not read by react-doctor at all.
	legacyLintConfig = ".eslintrc.json"
)

// skillLocations are the paths react-doctor's installer writes a skill into,
// for the agents this project is most likely to use.
var skillLocations = []string{
	filepath.Join(".claude", "skills", "react-doctor"),
	filepath.Join(".cursor", "skills", "react-doctor"),
	filepath.Join(".agents", "skills", "react-doctor"),
	filepath.Join(".opencode", "skills", "react-doctor"),
}

type manager int

const (
	managerNone manager = iota
	managerLefthook
	managerHusky
)

// hookManager reports which hook manager answers for this project.
//
// The question is which one responds, not which one a list expects: a project
// with husky has no use for a lefthook file, and writing one would create a
// configuration nothing reads.
func hookManager(p project.Project) manager {
	for _, name := range []string{"lefthook.yml", "lefthook.yaml", ".lefthook.yml", ".lefthook.yaml"} {
		if _, err := os.Stat(filepath.Join(p.Root, name)); err == nil {
			return managerLefthook
		}
	}
	if _, err := os.Stat(filepath.Join(p.Root, ".husky")); err == nil {
		return managerHusky
	}
	if p.LocalBinary("lefthook") != "" {
		return managerLefthook
	}
	return managerNone
}

// declaredKeys reports which of candidates the config at path declares.
//
// It tests the quoted key rather than the bare word, for the same reason its
// single-key predecessor already established: these files are JSONC, and a
// config that points at dharness carries a sentence like
// "Architecture boundaries live in the file dharness owns" in a comment. A
// bare substring answers yes to that sentence, which would make every
// correctly wired project report a conflict it does not have.
//
// A parser would be exact, and it is still not worth one: the product is
// stdlib-only, and a comment that quotes the key is a file written to defeat
// this check rather than an accident. A file that cannot be read declares
// nothing. Results come back in candidate order, so a prompt built from them
// reads the same way twice.
func declaredKeys(path string, candidates []string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var found []string
	for _, candidate := range candidates {
		if strings.Contains(string(raw), `"`+candidate+`"`) {
			found = append(found, candidate)
		}
	}
	return found
}

// fallowConfigCandidates are the fallow-recognised config names declaredKeys
// can read textually — fallow's own fallowConfigFiles list minus fallow.toml:
// TOML keys are bare, so the quoted-key test cannot answer for it. That is an
// accepted false negative (design decision 5 of framework-presets), not
// fixed by adding a TOML branch here.
var fallowConfigCandidates = []string{fallowConfig, ".fallowrc.jsonc"}

// fallowConfigPath names whichever of the project's own fallow configs
// exists, the same "which one responds" question hookManager answers for the
// hook manager. Empty when neither exists — declaredKeys already treats an
// unreadable path as "declares nothing", so an empty path is a safe input.
func fallowConfigPath(source string) string {
	for _, name := range fallowConfigCandidates {
		path := filepath.Join(source, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// declaredAt is declaredLine's stated location, no longer its stated value:
// the first 1-based line number that declares key in quoted form, or the
// documented sentinel 0 when key is not declared at all. Locating a key by a
// textual scan is sound — that part of declaredLine is kept exactly;
// *showing a value* with the same scan was defect 8 (a value spanning more
// than one line only ever showed its opening fragment), and declaredAt no
// longer tries. Feeds report.Declared.Line, whose own json tag omits a zero
// line rather than encoding it.
func declaredAt(path, key string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	needle := `"` + key + `"`
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	return 0
}

// gateInstalled reports whether git will actually run the gate, which is a
// different question from whether the configuration mentions it.
//
// The directory comes from git rather than from a join with ".git/hooks":
// a repository that sets core.hooksPath runs its hooks from elsewhere, and
// against the hardcoded path this answered false with the gate plainly
// installed. A repository git cannot answer for has no hooks directory to
// read, so it has no gate.
func gateInstalled(p project.Project) bool {
	hooks, err := project.HooksDir(p.Root)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(hooks, "pre-commit"))
	return err == nil && strings.Contains(string(raw), "lefthook")
}

func huskyWired(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
	return err == nil && strings.Contains(string(raw), gateCommand)
}

func appendHuskyGate(p project.Project, w *Writer) error {
	path := filepath.Join(p.Root, filepath.FromSlash(huskyHook))

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", huskyHook, err)
	}

	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return w.Write(path, []byte(body+gateCommand+"\n"))
}

// ensureShared appends an allow-list entry for name when .dharness/.gitignore
// does not already name it. Append rather than rewrite: the ignore file is
// written once, at adoption, and a repository adopted before this change
// keeps a list that predates every file added since. Rewriting it would
// discard whatever the project added to it, which is the one thing
// TestOwnedDirectoryKeepsAnExistingIgnoreFile exists to forbid — the same
// shape as appendHuskyGate, over a different file.
func ensureShared(p project.Project, w *Writer, name string) error {
	path := filepath.Join(p.Root, project.Dir, ".gitignore")

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	entry := "!" + name
	if strings.Contains(string(existing), entry) {
		return nil
	}

	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return w.Write(path, []byte(body+entry+"\n"))
}

// extendsWired reports whether a config already points at a dharness file.
//
// It looks for the reference rather than parsing, for the same reason the gate
// check does: dharness does not own these files and has no business
// understanding their structure to answer a yes-or-no question.
func extendsWired(dir, name, target string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	return err == nil && strings.Contains(string(raw), target)
}

// ownedFrom names a file dharness owns the way a config living in dir has to
// spell it.
//
// The owned directory sits at the repository root, because the gate it holds
// has to sit next to git. fallow's configuration lives with the JS project
// instead, so in a split layout it reaches the owned file by going up —
// `../.dharness/fallow.jsonc`. Both tools resolve `extends` relative to the
// file that declares it, so the reference is computed from the declaring
// directory rather than assumed to be the root.
func ownedFrom(p project.Project, dir, name string) string {
	owned := filepath.Join(p.Root, project.Dir, name)
	rel, err := filepath.Rel(dir, owned)
	if err != nil {
		return filepath.ToSlash(filepath.Join(project.Dir, name))
	}
	return filepath.ToSlash(rel)
}

// wireFallowExtends writes the reference. It runs only when
// fallowExtendsStep.Delegated(p) returned ok == false, which is only true
// when the project has no config of its own yet — the case where the project
// already owns .fallowrc.json is answered before Apply, not here.
// fallow's config belongs to the JS project, so it is written there.
func wireFallowExtends(p project.Project, w *Writer) error {
	target := ownedFrom(p, p.Source, ownedFallow)
	path := filepath.Join(p.Source, fallowConfig)
	return w.Write(path, fmt.Appendf(nil, "{\n  \"extends\": [%q]\n}\n", target))
}

// wireLefthookExtends does the same for the gate, which belongs to the
// repository because that is where git looks for it. Same precondition as
// wireFallowExtends: reached only when lefthookExtendsStep.Delegated(p)
// returned ok == false, which here means either no config at all or one
// lefthook scaffolded and nobody has written a key into yet.
//
// The scaffold's own text is kept below the reference. Delegated already
// established there is no key to merge with, so this is a write and not a
// merge — but what it writes over is lefthook documenting its own format to
// whoever opens the file, and deleting that buys nothing.
func wireLefthookExtends(p project.Project, w *Writer) error {
	target := ownedFrom(p, p.Root, ownedLefthook)
	path := filepath.Join(p.Root, lefthookConfig)

	body := fmt.Sprintf("extends:\n  - %s\n", target)
	if existing, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(existing)) != "" {
		body += "\n" + string(existing)
	}
	return w.Write(path, []byte(body))
}

// eslintFlatConfigNames are ESLint 9's flat config file names, tried in the
// order ESLint itself resolves them when a project somehow carries more
// than one.
var eslintFlatConfigNames = []string{"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs"}

// eslintTypeScriptConfigNames are the same shape written in TypeScript,
// which dharness never parses — the same reasoning §03's amendment already
// applies to Stryker configs: a second grammar plus typed helpers whose
// semantics belong to the project.
var eslintTypeScriptConfigNames = []string{"eslint.config.ts", "eslint.config.mts", "eslint.config.cts"}

// eslintLegacyConfigNames are the pre-flat-config ESLint file names, wider
// than legacyLintConfig — the one format legacyLintConfigStep adopts —
// because this question is "does any legacy config exist at all", not "is
// the one react-doctor reads broken".
var eslintLegacyConfigNames = []string{".eslintrc.json", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.yml", ".eslintrc.yaml", ".eslintrc"}

// eslintFlatConfig names the project's flat ESLint config path, or "" when
// none of the candidates exist — the same "which one responds" shape
// fallowConfigPath already answers for fallow's own config.
func eslintFlatConfig(source string) string {
	return firstExisting(source, eslintFlatConfigNames)
}

// eslintTypeScriptConfig names the project's TypeScript flat config path, or
// "" when none exists.
func eslintTypeScriptConfig(source string) string {
	return firstExisting(source, eslintTypeScriptConfigNames)
}

// eslintLegacyConfig names the project's legacy ESLint config path, or ""
// when none exists.
func eslintLegacyConfig(source string) string {
	return firstExisting(source, eslintLegacyConfigNames)
}

// firstExisting names the first of names that exists directly under dir, or
// "" when none do.
func firstExisting(dir string, names []string) string {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// wireEslintExtends writes a complete eslint.config.js that imports and
// spreads .dharness/eslint.config.js. It runs only when
// eslintExtendsStep.Delegated(p) returned ok == false with no ESLint config
// of any kind present — the write-if-absent case; splicing into an
// existing, understood config is a later slice's write path.
func wireEslintExtends(p project.Project, w *Writer) error {
	layers := preset.Layers(preset.Resolve(p))

	var b strings.Builder
	b.WriteString(eslintImportRegion(p, p.Source, layers, jsconfig.ESM, "", "\n"))
	b.WriteString("\n")
	b.WriteString("export default [\n")
	b.WriteString(eslintLayerRegion(layers, "  ", "\n"))
	b.WriteString("];\n")

	path := filepath.Join(p.Source, eslintConfig)
	return w.Write(path, []byte(b.String()))
}

// spliceEslintRegion is jsconfig.Splice, seamed only so a test can force an
// adversarial region through the real insert path and prove the candidate
// guard rolls the write back rather than let it through. Every real caller
// composes regions from eslintImportRegion/eslintLayerRegion, both of which
// always render well-formed JavaScript — there is no way to construct an
// ERROR-producing candidate through the public rendering functions alone.
var spliceEslintRegion = jsconfig.Splice

// SetEslintSpliceForTest replaces spliceEslintRegion and returns a restore
// function.
func SetEslintSpliceForTest(replacement func(src []byte, at int, region string) []byte) func() {
	previous := spliceEslintRegion
	spliceEslintRegion = replacement
	return func() { spliceEslintRegion = previous }
}

// spliceEslintConfig computes the candidate bytes for an eslint.config.js
// dharness has decided to edit directly — Delegated already answered
// ok == false for path, so this only ever sees a shape jsconfig.Analyze
// recognises: the insert path when neither marked region exists yet, the
// replace path when both already do. Both paths splice the later offset
// first so it does not shift the earlier one (design decision 6): the
// import region always precedes the layer region (Decision 5), so the layer
// splice or replace runs before the import one.
//
// A malformed marker pair is unreachable — Delegated refuses those before
// Apply or Satisfied ever call this. A mixed pair (one present, the other
// absent) is not malformed by markerRegion's own definition, but dharness
// never writes one marker pair without the other, so it can only mean a
// hand edit removed one region by itself; that is refused with an error
// rather than guessed at, the same reasoning the malformed case already
// uses.
//
// Shared by Satisfied, which compares the result against what is already on
// disk, and Apply, which verifies it and writes it — the two must never
// compute the render differently.
func spliceEslintConfig(p project.Project, path string, src []byte) ([]byte, error) {
	// Filtered against the very bytes being spliced, so what the factory
	// destructures and what this file spreads can never disagree about which
	// layers the project already brings in itself.
	layers := contributedLayers(preset.Layers(preset.Resolve(p)), src)
	dir := filepath.Dir(path)
	raw := string(src)
	module := jsconfig.ModuleOf(src)

	importFrom, importTo, importState := markerRegion(raw, eslintImportBegin, eslintImportEnd)
	layerFrom, layerTo, layerState := markerRegion(raw, eslintLayerBegin, eslintLayerEnd)

	switch {
	case importState == markersAbsent && layerState == markersAbsent:
		anchor, why, ok := jsconfig.Analyze(src)
		if !ok {
			return nil, fmt.Errorf("%s: %s", filepath.Base(path), why)
		}
		importRegion := eslintImportRegion(p, dir, layers, module, "", anchor.LineEnding)
		layerRegion := eslintLayerRegion(layers, anchor.Indent, anchor.LineEnding)

		candidate := spliceEslintRegion(src, anchor.LayerAt, layerRegion)
		candidate = spliceEslintRegion(candidate, anchor.ImportAt, importRegion)
		return candidate, nil

	case importState == markersPresent && layerState == markersPresent:
		importRegion := eslintImportRegion(p, dir, layers, module, regionIndent(raw, importFrom), regionLineEnding(raw, importFrom))
		layerRegion := eslintLayerRegion(layers, regionIndent(raw, layerFrom), regionLineEnding(raw, layerFrom))

		candidate := replaceRange(src, layerFrom, layerTo, layerRegion)
		candidate = replaceRange(candidate, importFrom, importTo, importRegion)
		return candidate, nil

	default:
		return nil, fmt.Errorf(
			"%s: the dharness:eslint-import and dharness:eslint-layer marker pairs disagree on whether they exist — one is present, the other is not, which dharness never writes and will not guess at",
			filepath.Base(path))
	}
}

// verifyEslintCandidate re-parses candidate the same way Analyze already
// parsed the source it was built from, and asserts the two invariants a
// splice must not break: no ERROR node anywhere in the default export, and
// exactly one well-formed region of each marker kind. There is no
// element-count assertion here — design decision 1 cuts it as redundant
// with these two, and wrong on the replace path, where the array's element
// count does not change.
//
// It runs against candidate, in memory, before anything is written: the
// resulting bytes are the candidate, so the unparseable file never exists
// on disk at all (design decision 6).
func verifyEslintCandidate(candidate []byte) error {
	if _, why, ok := jsconfig.Analyze(candidate); !ok {
		return fmt.Errorf("the resulting config would not parse: %s", why)
	}

	raw := string(candidate)
	if _, _, state := markerRegion(raw, eslintImportBegin, eslintImportEnd); state != markersPresent {
		return fmt.Errorf("the resulting config's dharness:eslint-import marker pair is not exactly one well-formed region")
	}
	if _, _, state := markerRegion(raw, eslintLayerBegin, eslintLayerEnd); state != markersPresent {
		return fmt.Errorf("the resulting config's dharness:eslint-layer marker pair is not exactly one well-formed region")
	}
	return nil
}
