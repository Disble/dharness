package setup

import (
	"bytes"
	_ "embed"
)

// coverageTiersAsset is the one managed source of dharness's native coverage
// configuration. `go:embed` keeps it in the binary rather than rebuilding it
// from a Go string literal, so what this repository reads and what a consumer
// receives are the same bytes by construction (§03: dharness owns the files it
// writes).
//
// The asset is JavaScript on purpose: the consumer's Vitest config is
// JavaScript, and the alternative — a Go-generated config — would make
// dharness the owner of a project's test configuration instead of a layer
// inside it.
//
//go:embed assets/coverage-tiers/runtime.mjs
var coverageTiersAsset []byte

// coverageTiersModule returns the managed runtime's bytes.
//
// The returned slice is a copy. A caller that trims or rewrites what it gets
// would otherwise corrupt the embedded asset for the rest of the process, and
// the failure would surface later as an unrelated consumer writing a truncated
// module.
func coverageTiersModule() []byte {
	return bytes.Clone(coverageTiersAsset)
}

// coverageTiersTypesAsset declares the runtime's one export for TypeScript.
// TypeScript resolves an import of coverage-tiers.mjs to coverage-tiers.d.mts
// beside it; without that file, a project that typechecks its Vitest config
// under `strict` fails with TS7016 on the import dharness spliced in, and the
// only way out is `allowJs` — widening the project's typecheck to fix ours.
// JSDoc inside the runtime cannot do this job: without `allowJs`, TypeScript
// never reads the .mjs at all.
//
//go:embed assets/coverage-tiers/runtime.d.mts
var coverageTiersTypesAsset []byte

// coverageTiersTypes returns the declaration's bytes, as a copy for the same
// reason coverageTiersModule's is.
func coverageTiersTypes() []byte {
	return bytes.Clone(coverageTiersTypesAsset)
}
