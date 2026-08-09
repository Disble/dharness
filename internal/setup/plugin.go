package setup

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
