// Managed by dharness: written by `dharness sync` beside coverage-tiers.mjs,
// whose only export it describes. Edits here are lost on the next sync.
//
// It exists so a project that typechecks its Vitest config under `strict`
// resolves the managed import without `allowJs` and without TS7016.

/** Where the coverage-tiers declaration lives, and where the JS project is. */
export interface CoverageTiersOptions {
  /** The directory holding coverage-tiers.json. */
  repositoryRoot: string;
  /** The JS project root, relative to repositoryRoot. Defaults to ".". */
  sourceRoot?: string;
}

/**
 * Wraps a Vitest config so that a complete coverage run measures the adopted
 * source at the tier each file was classified with.
 *
 * The config comes back in the shape it arrived in: an object stays an object,
 * and a sync or async config function stays one.
 */
export declare function withCoverageTiers<Config>(baseConfig: Config, options: CoverageTiersOptions): Config;
