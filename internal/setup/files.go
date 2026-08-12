package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// The files dharness owns, named after the tool whose format they hold.
const (
	ownedLefthook = "lefthook.yml"
	ownedFallow   = "fallow.jsonc"
	ownedRules    = "rules.json"
	ownedEslint   = "eslint.config.js"
)

// The files that belong to the project and gain at most one line.
const (
	lefthookConfig = "lefthook.yml"
	fallowConfig   = ".fallowrc.json"
	doctorConfig   = "doctor.config.json"
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

// declaredLine is a best-effort display of what the project itself wrote for
// key: the first line that declares it in quoted form, trimmed. A value
// spanning more than one line only shows its opening line — an honest limit
// of a textual technique that reads a line, not a parser that reads a value.
// The step this feeds is delegated either way, and the agent can open the
// file for the rest.
func declaredLine(path, key string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	needle := `"` + key + `"`
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// gateInstalled reports whether git will actually run the gate, which is a
// different question from whether the configuration mentions it.
func gateInstalled(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, ".git", "hooks", "pre-commit"))
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
// returned ok == false.
func wireLefthookExtends(p project.Project, w *Writer) error {
	target := ownedFrom(p, p.Root, ownedLefthook)
	path := filepath.Join(p.Root, lefthookConfig)
	return w.Write(path, fmt.Appendf(nil, "extends:\n  - %s\n", target))
}
