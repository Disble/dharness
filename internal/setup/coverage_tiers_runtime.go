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
