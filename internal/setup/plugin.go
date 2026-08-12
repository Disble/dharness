package setup

import (
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
	MaxFileLines int `json:"maxFileLines"`

	// RoleSuffixes are the file kinds whose contents are constrained: a
	// `.types.ts` declares types and nothing else.
	RoleSuffixes []string `json:"roleSuffixes"`
}

const thresholdsSchema = "dharness.rules/v1"

// DefaultThresholds is what a project starts with.
func DefaultThresholds() Thresholds {
	return Thresholds{
		Schema:       thresholdsSchema,
		MaxFileLines: 500,
		RoleSuffixes: []string{".types.ts", ".constants.ts", ".helpers.ts", ".schema.ts"},
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
