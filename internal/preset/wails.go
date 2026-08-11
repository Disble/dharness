package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Disble/dharness/internal/project"
)

// wailsJSONFile is the repository-root file Wails' own tooling writes.
const wailsJSONFile = "wails.json"

// wailsJSDirDefault is what Wails' own build command falls back to when
// wailsjsdir is absent — verified against Wails' own source
// (v2/pkg/commands/build/base.go: `options.WailsJSDir =
// filepath.Join(cwd, "frontend")`), not invented.
const wailsJSDirDefault = "frontend"

// wailsConfig is the one field this preset reads out of wails.json. Wails'
// own source names it WailsJSDir with json tag "wailsjsdir"
// (v2/internal/project/project.go); its SvelteKit guide instructs users to
// override it (`"wailsjsdir": "./frontend/src/lib"`), so a direct signal
// exists and no proxy is invented (§09).
type wailsConfig struct {
	WailsJSDir string `json:"wailsjsdir"`
}

// wails is the Wails preset: Root scope, because wails.json sits at the
// repository root, not beside the JS project it may be split from.
type wails struct{}

func (wails) ID() string   { return "wails" }
func (wails) Scope() Scope { return Root }

// Detect answers "is this a Wails project" from wails.json's presence alone
// — a project with none is not a Wails project, no match. Once that is
// settled, a read or decode failure is a different question: the project
// IS Wails, and this preset still contributes its documented default and
// names what it could not read in Match.Uncertain, rather than reading a
// parse failure as "not Wails" (the silent no-op this change exists to
// end). wails.json is plain JSON, not JSONC — Wails' own tooling writes
// it — so encoding/json is correct here.
func (wails) Detect(p project.Project) (Match, bool) {
	path := filepath.Join(p.Root, wailsJSONFile)
	if _, err := os.Stat(path); err != nil {
		return Match{}, false
	}
	// Evidence/Because are rendered text, not a filesystem operation — the
	// slash spelling keeps a golden fixture byte-stable across platforms,
	// matching golden_test.go's own substitutePaths (native and slash
	// spellings of p.Root are both replaced with <root>, in that order).
	display := filepath.ToSlash(path)

	dir := wailsJSDirDefault
	because := fmt.Sprintf("%s declares no \"wailsjsdir\", so the default Wails itself falls back to (%s/) applies", display, wailsJSDirDefault)
	uncertain := ""

	raw, err := os.ReadFile(path)
	switch {
	case err != nil:
		uncertain = fmt.Sprintf("%s exists but could not be read (%v); the documented default (%s/) was used instead", display, err, wailsJSDirDefault)
	default:
		var config wailsConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			uncertain = fmt.Sprintf("%s exists but does not parse as JSON, so its \"wailsjsdir\" could not be read; the documented default (%s/) was used instead", display, wailsJSDirDefault)
		} else if config.WailsJSDir != "" {
			dir = config.WailsJSDir
			because = fmt.Sprintf("%s declares \"wailsjsdir\": %q", display, config.WailsJSDir)
		}
	}

	return Match{
		ID:        "wails",
		Scope:     Root,
		Evidence:  fmt.Sprintf("%s present", display),
		Uncertain: uncertain,
		Manifest: Manifest{
			Schema: Schema,
			Facts: []Fact{{
				Key:     "ignorePatterns",
				Value:   []string{wailsIgnorePattern(p, dir)},
				Because: because,
			}},
		},
	}, true
}

// wailsIgnorePattern derives fallow's ignore pattern relative to
// p.Source — the directory the project's own fallow config lives in — from
// dir (wailsjsdir's value, or its documented default). Wails itself
// generates its JS bindings into <dir>/wailsjs, so
// filepath.Rel(p.Source, filepath.Join(p.Root, dir, "wailsjs")) computes
// exactly "wailsjs/**" on the motivating repository's split layout (design
// decision 9) — the check that the derivation is right.
func wailsIgnorePattern(p project.Project, dir string) string {
	source := p.Source
	if source == "" {
		// No JS project was detected at all; there is nothing more specific
		// than the repository root to be relative to.
		source = p.Root
	}

	target := filepath.Join(p.Root, dir, "wailsjs")
	rel, err := filepath.Rel(source, target)
	if err != nil {
		// filepath.Rel only fails when the two paths share no common base at
		// all (e.g. different drive letters on Windows) — nothing this
		// preset can resolve. The absolute path keeps the fact honest rather
		// than dropping it.
		rel = target
	}
	return filepath.ToSlash(rel) + "/**"
}
