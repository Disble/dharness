package preset

// registry is the presets Resolve consults, Root scope before Source (order
// within a scope is registry order, per resolve). generic always matches and
// stays last so a real signal is reported before the fallback would ever be
// reached — Resolve/resolve don't rely on this ordering (a matching preset
// short-circuits nothing), but it keeps the list read the way it resolves.
//
// Split out of preset.go into its own file now that four presets exist: this
// is the "one switch in a factory" design decision 1 names, worth separating
// once it stopped being a two-line list next to Resolve/resolve/Keys/Seeds.
var registry = []Preset{wails{}, nextjs{}, expo{}, generic{}}
