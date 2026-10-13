package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// Everything that consumes the rules package — the install step, the
// react-doctor declaration, the thresholds file — reads these constants, so
// the whole contract with it is this one file.
//
// The prefix comes from the plugin's own `meta.name`, not from the package
// name. Verified by running: a rule declared as `dharness/max-file-lines`
// resolves when `meta.name` is `dharness`, whatever the package is called.
const (
	// RulesPackage is what gets installed and named in `plugins`.
	RulesPackage = "dharness-eslint-plugin"

	// RulesPrefix is the plugin's meta.name, which every rule id is scoped by.
	RulesPrefix = "dharness"
)

// Rules are the checks that exist because no other tool in the harness does
// them: react-doctor has 787 rules and none of these, fallow bans names rather
// than shapes, and oxlint has neither `no-restricted-syntax` nor a jsdoc rule.
//
// They are the guardrails on generated code. Without a limit a model will
// produce a three-thousand-line file holding types, helpers, constants and a
// class, with nothing documented.
var Rules = []string{
	"max-file-lines",
	"require-jsdoc",
	"require-variable-jsdoc",
	"role-file-shape",
	"folder-ownership",
	"pure-index-barrel",
}

// Thresholds are the numbers the rules read from `.dharness/rules.json`.
//
// They live in a file rather than in the rule declaration because react-doctor
// accepts only `error`, `warn` or `off` as a severity — `["error", 500]` is
// rejected outright and `context.options` arrives empty. Without this file the
// limit would be compiled into the package and changing it in one project
// would mean publishing a new version.
type Thresholds struct {
	Schema string `json:"schema"`

	// MaxFileLines is the ceiling a single file may not cross.
	//
	// A float64, not an int, because the plugin validates this field with
	// `Number.isInteger(value) && value > 0` and a JS number has no integer
	// ceiling above it. An int would refuse a valid ceiling somewhere above
	// 2^63 — a bound the plugin does not have, so an invented one — and the
	// file would be silently rewritten to the default. Whole numbers render
	// exactly as before (`500` stays `500`); a value above 2^53 loses the
	// precision the plugin's own reader loses, because both sides parse a
	// JSON number into the same IEEE-754 double.
	MaxFileLines float64 `json:"maxFileLines"`

	// RoleSuffixes are the file kinds whose contents are constrained: a
	// `.types.ts` declares types and nothing else.
	RoleSuffixes []string `json:"roleSuffixes"`

	// CountComments decides whether a line that is only a comment counts
	// toward MaxFileLines. False counts code only — the default, because a
	// ceiling exists to bound what a reader has to hold, and generated
	// commentary is not what overruns a file.
	CountComments bool `json:"countComments"`

	// CountBlankLines decides whether blank lines count toward
	// MaxFileLines. False by default, for CountComments' reason.
	CountBlankLines bool `json:"countBlankLines"`
}

const thresholdsSchema = "dharness.rules/v1"

// DefaultThresholds is what a project starts with.
func DefaultThresholds() Thresholds {
	return Thresholds{
		Schema:          thresholdsSchema,
		MaxFileLines:    500,
		RoleSuffixes:    []string{".types.ts", ".constants.ts", ".helpers.ts", ".schema.ts"},
		CountComments:   false,
		CountBlankLines: false,
	}
}

// RuleIDs returns the fully scoped ids, which is how react-doctor names them.
func RuleIDs() []string {
	ids := make([]string, 0, len(Rules))
	for _, rule := range Rules {
		ids = append(ids, RulesPrefix+"/"+rule)
	}
	return ids
}

// DefaultSeverity is what dharness writes for a rule the project has not
// chosen a severity for itself (§05) — its sole caller is
// ownedEslintConfig, which calls it for every rule on every run: it accepts
// either the bare name or the scoped id.
//
// folder-ownership requires that a split module publish an `index.ts`, so a
// project that deliberately has no barrel files cannot satisfy it — every
// split module reports, forever, and the rule is not wrong so much as not
// that project's architecture. Measured on one such repository: eight
// findings, none actionable, sitting alongside real ones. Whether a project
// publishes through barrels is observable from the tree rather than a
// property of its framework, so the answer comes from p.PublishesBarrels()
// (asked of git) instead of a global constant: dharness writes "error"
// where the tree has at least one barrel, "off" where it has none.
//
// This runs on every sync now, not once at first adoption. The
// first-write-only limit this function used to carry was never a property
// of the derivation above — it was a property of writing into
// doctor.config.json, a file the project also edited, where dharness could
// not tell "the project chose off" from "dharness wrote off" (§05), so
// re-deriving risked overwriting a deliberate choice. That file is gone;
// .dharness/eslint.config.js is dharness's alone, regenerated whole by this
// function every run, so there is nothing inside it left for a project to
// have "chosen" — a project that wants a different severity states it in
// its own eslint.config.js, which is not dharness's to overwrite. The limit
// retired with the file it was a property of, not with this function.
//
// pure-index-barrel stays "error" unconditionally: it constrains a barrel
// that exists rather than requiring one, so in a project without barrels it
// never fires. Inert is not the same as false.
//
// ArchitecturePrompt says how to turn folder-ownership back on, because a
// default nobody is told about is indistinguishable from the rule not
// existing.
func DefaultSeverity(p project.Project, rule string) string {
	switch strings.TrimPrefix(rule, RulesPrefix+"/") {
	case "folder-ownership":
		if p.PublishesBarrels() {
			return "error"
		}
		return "off"
	}
	return "error"
}

// ------------------------------------------------ the thresholds document
//
// Everything below is the contract with the plugin's own src/thresholds.ts.
// It reads `.dharness/rules.json` and validates each field exactly the way
// this file does, so the two must agree about what a valid document is: a
// value this side accepts and the plugin rejects would leave a project with
// a file it believes it set and a rule that ignores it.

// recognizedThresholdKeys is exactly what the plugin's ThresholdsFile reads.
// Every other key in the document belongs to the project and is never
// touched — a field this side does not know about is not this side's to
// delete.
var recognizedThresholdKeys = map[string]bool{
	"schema":          true,
	"maxFileLines":    true,
	"roleSuffixes":    true,
	"countComments":   true,
	"countBlankLines": true,
}

// thresholdsFile is what `.dharness/rules.json` amounts to on one run.
//
// The three fields are one answer and are never read apart: a file dharness
// will write carries raw and document; a file it must leave alone carries
// repair and neither. Keeping them together is what makes "compare, then
// write" a single derivation instead of two reads that could disagree — the
// same reason eslintExtendsStep.Satisfied and eslintConfigLoads share one
// function.
//
// document is derived from the file itself, never from a cached parse: §07's
// rule is that state is read from the repository, and a remembered answer is
// exactly the file that can lie.
type thresholdsFile struct {
	// raw is the file exactly as it was found, nil when it does not exist.
	raw []byte

	// document is the normalized document dharness writes and compares
	// against. Nil when repair is non-empty.
	document []byte

	// repair names why a file that exists must be left byte-for-byte alone,
	// and what the agent does about it instead.
	repair string
}

// readThresholdsFile derives what dharness will do with the thresholds file
// at path.
//
// A missing file is the defaults: a project that never decided is not a
// project that is broken, the same rule the plugin's own reader applies. A
// file that exists is either normalized into the document dharness would
// write — field by field, the same validation the plugin applies — or named
// as the agent's to repair.
func readThresholdsFile(path string) thresholdsFile {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return thresholdsFile{document: renderThresholds(DefaultThresholds(), nil)}
	}
	if err != nil {
		return thresholdsFile{repair: unreadableThresholdsFile(err)}
	}

	document, repair := normalizeThresholds(raw)
	return thresholdsFile{raw: raw, document: document, repair: repair}
}

// normalizeThresholds turns the bytes of a rules file into the document
// dharness writes there, or the reason it leaves them alone.
//
// A document that is not a JSON object is not one field's problem: there is
// no field to repair and no way to tell which parts were meant, so it is
// returned as unreadable rather than guessed at (§05).
func normalizeThresholds(raw []byte) (document []byte, repair string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, unreadableThresholdsDocument()
	}

	thresholds := DefaultThresholds()
	if value := positiveInteger(fields["maxFileLines"]); value != nil {
		thresholds.MaxFileLines = *value
	}
	if value := stringList(fields["roleSuffixes"]); value != nil {
		thresholds.RoleSuffixes = value
	}
	if value := booleanValue(fields["countComments"]); value != nil {
		thresholds.CountComments = *value
	}
	if value := booleanValue(fields["countBlankLines"]); value != nil {
		thresholds.CountBlankLines = *value
	}

	unknown := make(map[string]json.RawMessage)
	for key, value := range fields {
		if recognizedThresholdKeys[key] {
			continue
		}
		unknown[key] = value
	}

	return renderThresholds(thresholds, unknown), ""
}

// renderThresholds marshals the thresholds document the way the file has
// always looked: the recognized keys in a fixed order, then whatever keys a
// project added, sorted. A hand-edited file therefore survives normalization
// with only its whitespace canonicalized, and two runs over one tree produce
// the same bytes (§07, §15).
func renderThresholds(t Thresholds, unknown map[string]json.RawMessage) []byte {
	known := []struct {
		key   string
		value any
	}{
		{"schema", t.Schema},
		{"maxFileLines", t.MaxFileLines},
		{"roleSuffixes", t.RoleSuffixes},
		{"countComments", t.CountComments},
		{"countBlankLines", t.CountBlankLines},
	}

	order := make([]string, 0, len(unknown))
	for key := range unknown {
		order = append(order, key)
	}
	sort.Strings(order)

	var b bytes.Buffer
	b.WriteString("{\n")
	first := true
	for _, field := range known {
		appendThresholdField(&b, &first, field.key, field.value)
	}
	for _, key := range order {
		appendThresholdField(&b, &first, key, unknown[key])
	}
	b.WriteString("\n}\n")
	return b.Bytes()
}

// appendThresholdField writes one `"key": value` pair indented the way
// json.MarshalIndent indents a struct, so a document with no unknown keys is
// byte-identical to what this file produced before unknown keys existed.
//
// The marshals cannot fail — the values are a string, an int, two bools, a
// []string, and RawMessages a successful json.Unmarshal already validated —
// so their errors are dropped rather than turned into branches no input
// reaches (ourDeclared's own precedent for json.Marshal of a string).
func appendThresholdField(b *bytes.Buffer, first *bool, key string, value any) {
	encoded, _ := json.MarshalIndent(value, "  ", "  ")
	quoted, _ := json.Marshal(key)

	if !*first {
		b.WriteString(",\n")
	}
	*first = false
	b.WriteString("  ")
	b.Write(quoted)
	b.WriteString(": ")
	b.Write(encoded)
}

// positiveInteger mirrors the plugin's own check: a whole number greater
// than zero. Zero, a negative and a fractional value are typos, not
// decisions, and each one falls back to the default alone.
//
// It returns a float64 rather than an int because the plugin's check is
// `Number.isInteger(value) && value > 0`, which has no upper bound: 1e19 and
// 1e308 are whole numbers a rule honours. Narrowing to int would mint a
// ceiling of its own. A literal no float64 can hold (1e309) makes
// json.Unmarshal report an out-of-range error before this reads it, which is
// the plugin's own answer for the same input: Infinity is not an integer.
func positiveInteger(raw json.RawMessage) *float64 {
	if raw == nil {
		return nil
	}

	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	if value < 1 || value != math.Trunc(value) {
		return nil
	}

	return &value
}

// stringList mirrors the plugin's own check: a list where every entry is a
// string. One non-string entry means the file was hand-edited wrong, and
// refusing the whole list is what the plugin does — a partially valid list
// would silently drop the entries it could not read.
func stringList(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	suffixes, ok := decoded.([]any)
	if !ok {
		return nil
	}

	list := make([]string, 0, len(suffixes))
	for _, entry := range suffixes {
		text, ok := entry.(string)
		if !ok {
			return nil
		}
		list = append(list, text)
	}
	return list
}

// booleanValue mirrors the plugin's own check: only a JSON boolean decides a
// counting policy. `null`, `0`, `""`, the string `"true"` and the number 1
// are not decisions, and truthiness would turn the last two into one.
func booleanValue(raw json.RawMessage) *bool {
	if raw == nil {
		return nil
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return &value
}

// unreadableThresholdsDocument is the reason a present rules.json dharness
// will not rewrite: it is not a JSON object, so there is no field to repair
// and no way to tell what its author meant.
//
// It names the file and the exact repair, because the reader is the agent
// that ran the commit, not a person dharness can see (§21).
func unreadableThresholdsDocument() string {
	return fmt.Sprintf(
		"%s is not a JSON object dharness can read, so dharness cannot tell which\nparts were meant and does not rewrite it. Repair it into a JSON object — the\nfields the line-count rules read are `maxFileLines`, `roleSuffixes`,\n`countComments` and `countBlankLines` — and re-run `dharness sync`.",
		thresholdsRelPath())
}

// unreadableThresholdsFile is the reason a rules.json dharness could not
// open is left alone: overwriting a file it cannot see would destroy the
// only copy of what the project tried to decide.
func unreadableThresholdsFile(err error) string {
	return fmt.Sprintf(
		"%s could not be read: %s. dharness does not overwrite a file it cannot see;\nrepair or replace it and re-run `dharness sync`.",
		thresholdsRelPath(), err)
}

// thresholdsRelPath names the thresholds file the way a report reader reads
// it: under the directory dharness owns, never as an absolute path.
func thresholdsRelPath() string {
	return filepath.ToSlash(filepath.Join(project.Dir, ownedRules))
}
