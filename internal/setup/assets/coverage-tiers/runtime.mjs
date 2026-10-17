// Managed by dharness. Do not edit: the next `dharness sync` writes this file
// again from the binary it ships in.
//
// This module is the native Vitest side of the coverage tiers. The
// classification and the numbers are never decided here — they come from
// `dharness policy --root <repositoryRoot>`, which resolves
// `coverage-tiers.json` once and answers with the complete adopted universe.
// This file only projects that answer into a Vitest config: it copies tiers
// into thresholds, projects paths onto the project root, and appends one
// reporter. There is no glob matching and no tier table here on purpose, since
// a second classifier in JavaScript is exactly the drift the Go resolver
// exists to prevent.
//
// Native assumptions. They are what this file depends on, not what it proves;
// each one is measured against the real runner by the gate work:
//
//   - `test.coverage.include` decides which files are measured, and a file that
//     is listed there and imported by no test is reported at 0%. That is what
//     makes an adopted unit's old, unimported source enforceable.
//   - `test.coverage.thresholds` accepts a per-path entry holding the four
//     metrics and `perFile: true`, which is the shape the approved example
//     declares.
//   - A reporter instance registered in `test.reporters` receives
//     `onCoverage(coverage)` after the builtin provider collected coverage and
//     before the provider's own reporters print. Without coverage the hook is
//     never called, which is why nothing here touches an ordinary run.
//   - That coverage object exposes the measured files, either through
//     `files()` (the CoverageMap shape) or as a file map keyed by path.
//   - The command line is the runner's to interpret, in every form it accepts
//     (`--coverage`, `--coverage=false`, `--coverage.enabled false`), and its
//     own `vitest/node` `parseCLI` is the one authority this module asks. An
//     explicit `false` on the command line disables coverage even when the
//     config enabled it, and an option that says nothing about `enabled`
//     leaves the config's own value in force. This module never writes
//     `enabled` itself: it never turns coverage on, so an ordinary, focused or
//     watch run stays coverage-free until the complete gate or CI asks (§S2).
//
// Limits kept in the open rather than papered over:
//
//   - Reading the command line requires the runner to be resolvable from the
//     project root (`vitest/node`). A project whose Vitest is missing, or too
//     old to expose `parseCLI`, is refused with the fix named; this module
//     never falls back to reading the flags itself, because a second grammar
//     here would drift from the runner the moment either changed.
//   - Paths are projected between the repository root and the project root and
//     nothing else. A consumer glob in `include` is preserved but never
//     interpreted, so this module cannot tell whether that glob also measures
//     unadopted code — the consumer's own scope stays the consumer's decision.
//   - A relative path in the coverage map is resolved against the process's
//     working directory, which is the runner's own root.
//   - `dharness` is spawned without a shell, so on Windows it must be an
//     executable the runtime can start directly rather than a shell-only shim.

import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import path from 'node:path';

// The coverage integration helper, never a runner: this module resolves tiers
// and never invokes Vitest or Jest itself.
const POLICY_BINARY = 'dharness';
const POLICY_SCHEMA_VERSION = 1;

// The builtin providers Vitest ships, and the package each one needs. A
// provider this module does not know is left to the runner to report.
const PROVIDER_PACKAGES = {
  v8: '@vitest/coverage-v8',
  istanbul: '@vitest/coverage-istanbul',
};
const DEFAULT_PROVIDER = 'v8';

// The four metrics every resolved tier declares.
const METRICS = ['statements', 'branches', 'functions', 'lines'];

// Reporter dharness appends its check beside, so naming no reporter never
// means losing the runner's own output.
const DEFAULT_REPORTER = 'default';

// How many missing files a refusal names before it counts the rest.
const MISSING_LISTED = 5;

/**
 * Wraps a Vitest config so that a complete coverage run measures exactly the
 * adopted universe the policy resolved, at the tier each file was classified
 * with.
 *
 * The config arrives as an object, a function or an async function, and comes
 * back in the same shape: a sync callable stays sync, and a config object stays
 * an object. Nothing is added, resolved or required until the run actually
 * asks for coverage.
 */
export function withCoverageTiers(baseConfig, options) {
  const resolved = normalizeOptions(options);
  if (typeof baseConfig === 'function') {
    return function coverageTiersConfig(...args) {
      const config = baseConfig(...args);
      if (isThenable(config)) {
        return config.then((value) => applyCoverageTiers(value, resolved));
      }
      return applyCoverageTiers(config, resolved);
    };
  }
  return applyCoverageTiers(baseConfig, resolved);
}

function normalizeOptions(options) {
  if (!isObject(options)) {
    throw new Error('coverage tiers: withCoverageTiers requires an options object { repositoryRoot, sourceRoot }');
  }
  const { repositoryRoot, sourceRoot = '.' } = options;
  if (typeof repositoryRoot !== 'string' || repositoryRoot.trim() === '') {
    throw new Error('coverage tiers: withCoverageTiers requires repositoryRoot, the directory holding coverage-tiers.json');
  }
  if (typeof sourceRoot !== 'string' || sourceRoot.trim() === '') {
    throw new Error('coverage tiers: sourceRoot must be a path relative to repositoryRoot, or "."');
  }
  const repository = path.resolve(repositoryRoot);
  const project = path.resolve(repository, sourceRoot);
  const outside = path.relative(repository, project);
  if (outside === '..' || outside.startsWith('..' + path.sep) || path.isAbsolute(outside)) {
    throw new Error('coverage tiers: sourceRoot ' + JSON.stringify(sourceRoot) + ' escapes repositoryRoot; the project root must be the repository root or a directory inside it');
  }
  return { repositoryRoot: repository, projectRoot: project };
}

function applyCoverageTiers(baseConfig, options) {
  assertConfig(baseConfig);
  if (!coverageRequested(baseConfig, options.projectRoot)) {
    return baseConfig;
  }
  const resolution = resolvePolicy(options.repositoryRoot);
  if (!resolution.active || resolution.files.length === 0) {
    // No declaration is not a failure: a repository that never classified its
    // source keeps the config it wrote (§ acceptance criteria: unadopted
    // legacy behavior is preserved).
    return baseConfig;
  }
  assertProviderAvailable(baseConfig, options.projectRoot);
  return mergeTiers(baseConfig, resolution, options);
}

function assertConfig(baseConfig) {
  if (!isObject(baseConfig) || Array.isArray(baseConfig) || isThenable(baseConfig)) {
    throw new Error('coverage tiers: withCoverageTiers accepts a Vitest config object or a config function returning one; pass the result of defineConfig(...)');
  }
}

/**
 * Whether this process is a complete coverage run. The answer is the runner's
 * own, and this module never reconstructs it: a config that enabled coverage is
 * honored when the command line says nothing about coverage, and once the
 * command line does mention coverage, what it asked for is read by the installed
 * Vitest. Reading the flags here by hand is what left a separate value such as
 * `--coverage.enabled false` unseen and let a config that enabled coverage
 * ignore an explicit `false` beside it.
 */
function coverageRequested(baseConfig, projectRoot) {
  const coverage = baseConfig.test && baseConfig.test.coverage;
  const configured = coverage === undefined || coverage === null ? undefined : coverage.enabled;
  if (!commandLineMentionsCoverage(process.argv)) {
    return configured === true;
  }
  return nativeCoverageRequested(projectRoot, configured);
}

/**
 * Non-semantic recognition, deliberately: it answers only whether the command
 * line says anything about coverage at all, so an ordinary focused or watch run
 * never has to load the runner. It reads no value and applies no precedence —
 * `true`, `false` and "no value" are the native parser's to decide.
 */
function commandLineMentionsCoverage(argv) {
  for (const argument of argv) {
    if (argument === '--coverage' || argument.startsWith('--coverage=') || argument.startsWith('--coverage.')) {
      return true;
    }
  }
  return false;
}

/**
 * The runner's own `parseCLI` decides what the command line asked for, and an
 * option that does not mention `enabled` leaves the config's value in force. The
 * parser returns the container `{ filter, options }` and the coverage options
 * live under `options`; reading `parsed.coverage` reads nothing, which is
 * exactly how an explicit `false` was lost and a disabled run still resolved the
 * policy. The parser mutates the array it is handed, so it receives a fresh one.
 */
function nativeCoverageRequested(projectRoot, configured) {
  const parseCLI = loadVitestParseCLI(projectRoot);
  const parsed = parseCLI(['vitest', ...process.argv.slice(2)]);
  const options = isObject(parsed) ? parsed.options : undefined;
  if (!isObject(options) || !isObject(options.coverage)) {
    return configured === true;
  }
  const requested = options.coverage.enabled;
  return requested === undefined ? configured === true : requested === true;
}

/**
 * The installed Vitest is the parser, never a copy of one. A runner that cannot
 * be resolved, or that does not expose `parseCLI`, is named with its fix rather
 * than replaced by a hand-written grammar this module would then own.
 */
function loadVitestParseCLI(projectRoot) {
  let vitest;
  try {
    vitest = createRequire(path.join(projectRoot, 'package.json'))('vitest/node');
  } catch (error) {
    const detail = error && error.message ? error.message : String(error);
    throw new Error('coverage tiers: the installed Vitest API "vitest/node" could not be loaded from ' + projectRoot + ' (' + detail + '); the coverage command line must be read by Vitest itself, so install vitest where this project can resolve it and run again — dharness never reads the flags on its own and never downloads the runner');
  }
  if (!isObject(vitest) || typeof vitest.parseCLI !== 'function') {
    throw new Error('coverage tiers: the installed Vitest at ' + projectRoot + ' does not expose parseCLI from "vitest/node"; this module reads the coverage command line with the runner it integrates with, so upgrade or reinstall vitest in this project instead of letting dharness guess the flags');
  }
  return vitest.parseCLI;
}

/**
 * The one authoritative answer, asked of the Go resolver in this project. The
 * command is executed directly, without a shell, and never installed or
 * fetched here.
 */
function resolvePolicy(repositoryRoot) {
  let raw;
  try {
    raw = execFileSync(POLICY_BINARY, ['policy', '--root', repositoryRoot], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    });
  } catch (error) {
    throw new Error(policyFailureMessage(repositoryRoot, error));
  }
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new Error('coverage tiers: "dharness policy --root ' + repositoryRoot + '" did not return JSON: ' + error.message);
  }
  return assertResolution(parsed, repositoryRoot);
}

function policyFailureMessage(repositoryRoot, error) {
  const command = POLICY_BINARY + ' policy --root ' + repositoryRoot;
  if (error && error.code === 'ENOENT') {
    return 'coverage tiers: "' + POLICY_BINARY + '" is not on the PATH, so the coverage tiers cannot be resolved; install dharness and run "' + command + '" from the repository';
  }
  if (error && typeof error.status !== 'number' && error.code) {
    return 'coverage tiers: "' + command + '" could not be started (' + error.code + '); ' + POLICY_BINARY + ' must be an executable the runtime can start directly, not a shell-only shim';
  }
  const stderr = error && typeof error.stderr === 'string' ? error.stderr.trim() : '';
  const detail = stderr === '' ? String(error && error.message ? error.message : error) : stderr;
  const status = error && typeof error.status === 'number' ? ' (exit ' + error.status + ')' : '';
  return 'coverage tiers: "' + command + '" failed' + status + ': ' + detail;
}

/**
 * Refuses a policy answer this module does not understand. An answer it cannot
 * read must never become an empty projection, because an empty projection is a
 * gate that measures nothing and says it passed.
 */
function assertResolution(value, repositoryRoot) {
  const source = '"dharness policy --root ' + repositoryRoot + '"';
  if (!isObject(value) || value.version !== POLICY_SCHEMA_VERSION || typeof value.active !== 'boolean' || !Array.isArray(value.files)) {
    throw new Error('coverage tiers: ' + source + ' returned an unexpected schema; expected {"version":1,"active":boolean,"files":[{"path","tier","coverage"}]}');
  }
  for (const file of value.files) {
    if (!isObject(file)
      || typeof file.path !== 'string' || file.path === ''
      || typeof file.tier !== 'string'
      || typeof file.coverage !== 'number' || !Number.isFinite(file.coverage)) {
      throw new Error('coverage tiers: ' + source + ' returned a file entry without {path, tier, coverage}');
    }
  }
  // JSON.parse produced this private value; validation need not clone it or
  // coerce the boolean it just checked. The Go resolver still owns every tier.
  return value;
}

/**
 * The provider is a prerequisite of the run the consumer asked for, and the
 * only correct place to satisfy it is the project's own install: this module
 * reports the missing package and its command, and downloads nothing.
 *
 * It is checked only once coverage was requested, so a project that has not
 * installed a provider can still run its ordinary tests.
 */
function assertProviderAvailable(baseConfig, projectRoot) {
  const coverage = (baseConfig.test && baseConfig.test.coverage) || {};
  const provider = coverage.provider === undefined ? DEFAULT_PROVIDER : coverage.provider;
  if (typeof provider !== 'string') {
    // A provider instance is the consumer's own object; there is no package
    // name to resolve and nothing to refuse.
    return;
  }
  const packageName = PROVIDER_PACKAGES[provider];
  if (packageName === undefined) {
    return;
  }
  try {
    createRequire(path.join(projectRoot, 'package.json')).resolve(packageName);
  } catch (error) {
    throw new Error('coverage tiers: the "' + provider + '" coverage provider needs "' + packageName + '", which is not installed in ' + projectRoot + '; install it locally before running coverage (for example: bun add -d ' + packageName + ') — dharness never downloads it at run time');
  }
}

function mergeTiers(baseConfig, resolution, options) {
  const test = { ...(baseConfig.test || {}) };
  const coverage = { ...(test.coverage || {}) };
  const include = consumerInclude(coverage.include);
  // The Set answers membership; the array keeps the consumer's order and is
  // what Vitest reads. Both grow together, so neither can drift.
  const included = new Set(include);
  const thresholds = { ...consumerThresholds(coverage.thresholds) };
  const required = [];
  for (const file of resolution.files) {
    const projectPath = projectFilePath(file.path, options);
    thresholds[projectPath] = mergeThreshold(projectPath, thresholds[projectPath], file.coverage);
    if (!included.has(projectPath)) {
      included.add(projectPath);
      include.push(projectPath);
    }
    if (file.coverage > 0) {
      // The threshold and include entries above are the project's coordinate,
      // because that is where Vitest reads them. Presence is a different
      // question asked in a different coordinate: the policy already speaks
      // repository-relative forward-slash paths, which is exactly what
      // reportedSourceSet produces. In a split layout — repository root above
      // the project root — the two are different strings for the same file,
      // so carrying the project's path here would refuse a file that was
      // measured.
      required.push(file.path);
    }
  }
  coverage.include = include;
  coverage.thresholds = thresholds;
  test.coverage = coverage;
  if (required.length > 0) {
    test.reporters = withChecker(test.reporters, required, options.repositoryRoot);
  }
  return { ...baseConfig, test };
}

/**
 * A path is projected, never matched. The policy speaks repository-relative
 * paths and Vitest resolves its config against the project root, so this is
 * the one translation between them.
 */
function projectFilePath(repositoryPath, options) {
  const absolute = path.resolve(options.repositoryRoot, repositoryPath);
  const relative = path.relative(options.projectRoot, absolute);
  if (relative === '' || relative === '..' || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) {
    throw new Error('coverage tiers: adopted source ' + repositoryPath + ' is outside this project root (' + options.projectRoot + '); a test project only measures its own root, so point sourceRoot at the shared root of the adopted prefixes or narrow the prefix this project owns — declaring the tiers and not enforcing them here is not a silent option');
  }
  return relative.split(path.sep).join('/');
}

function consumerInclude(include) {
  if (include === undefined) {
    return [];
  }
  if (typeof include === 'string') {
    return [include];
  }
  if (Array.isArray(include)) {
    return include.slice();
  }
  throw new Error('coverage tiers: test.coverage.include must be a list of globs for the adopted sources to be added to it; found ' + describe(include));
}

function consumerThresholds(thresholds) {
  if (thresholds === undefined) {
    return {};
  }
  if (isObject(thresholds) && !Array.isArray(thresholds)) {
    return thresholds;
  }
  throw new Error('coverage tiers: test.coverage.thresholds must be an object so resolved per-file tiers can merge into it; found ' + describe(thresholds));
}

/**
 * The consumer's existing entry is the starting point, and the resolved tier
 * fills it in. A metric the consumer set higher is kept as written, and a
 * metric the consumer set lower stops the run: lowering the resolved tier
 * would enforce nothing, and raising theirs would overwrite a decision that
 * belongs to the project (§05).
 */
function mergeThreshold(filePath, existing = {}, coverage) {
  if (!isObject(existing) || Array.isArray(existing)) {
    throw new Error('coverage tiers: test.coverage.thresholds[' + JSON.stringify(filePath) + '] must be an object to merge the resolved tier into; found ' + describe(existing));
  }
  const merged = { ...existing };
  for (const metric of METRICS) {
    const declared = existing[metric];
    if (declared === undefined) {
      merged[metric] = coverage;
      continue;
    }
    if (typeof declared !== 'number' || !Number.isFinite(declared)) {
      throw new Error('coverage tiers: test.coverage.thresholds[' + JSON.stringify(filePath) + '].' + metric + ' must be a number; found ' + describe(declared));
    }
    if (declared < coverage) {
      throw new Error('coverage tiers: conflict on ' + filePath + ': the resolved tier requires ' + metric + ' >= ' + coverage + ', and the consumer declared ' + declared + '; raise the threshold to the declared tier in coverage-tiers.json, or raise the consumer value — dharness never resolves this by overwriting either one');
    }
  }
  if (existing.perFile === false) {
    throw new Error('coverage tiers: test.coverage.thresholds[' + JSON.stringify(filePath) + '].perFile is false, and an adopted source is enforced per file; remove the entry and let the resolved tier declare it, or change the classification in coverage-tiers.json');
  }
  merged.perFile = true;
  return merged;
}

/**
 * The runner's own reporters are kept, and the adopted-source check is
 * appended beside them. Replacing that list would be the quietest way to lose
 * the coverage table a person is supposed to read.
 */
function withChecker(reporters, required, repositoryRoot) {
  const checker = new AdoptedSourceReporter(required, repositoryRoot);
  if (reporters === undefined) {
    return [DEFAULT_REPORTER, checker];
  }
  if (typeof reporters === 'string') {
    return [reporters, checker];
  }
  if (Array.isArray(reporters)) {
    return [...reporters, checker];
  }
  throw new Error('coverage tiers: test.reporters must be a reporter name or a list of reporters so the adopted-source check can be appended; found ' + describe(reporters));
}

/**
 * Presence, never a percentage.
 *
 * The provider supplies the file map, and this check asks one question of it:
 * is every adopted source the policy requires with a threshold actually in the
 * report? A consumer's `exclude` can drop an adopted file from the report
 * while its threshold sits in the config unmeasured, which would read as
 * compliance. The tiers themselves are still the runner's thresholds; nothing
 * here recomputes a metric.
 */
class AdoptedSourceReporter {
  constructor(required, repositoryRoot) {
    this.required = required;
    this.repositoryRoot = repositoryRoot;
  }

  onCoverage(coverage) {
    const reported = reportedSourceSet(coverage, this.repositoryRoot);
    const missing = this.required.filter((file) => !reported.has(file));
    if (missing.length > 0) {
      throw new Error(missingSourceMessage(missing, this.repositoryRoot));
    }
  }
}

function reportedSourceSet(coverage, repositoryRoot) {
  const reported = new Set();
  for (const file of coverageFiles(coverage)) {
    if (typeof file !== 'string' || file === '') {
      continue;
    }
    const relative = path.relative(repositoryRoot, path.resolve(file));
    // Only canonical adopted paths are queried in this private set. Root and
    // outside-root entries cannot match them, so filtering those again adds no
    // enforcement. Keep relative normalization for Windows volume spelling.
    reported.add(relative.split(path.sep).join('/'));
  }
  return reported;
}

function coverageFiles(coverage) {
  if (!isObject(coverage)) {
    return [];
  }
  if (typeof coverage.files === 'function') {
    const files = coverage.files();
    return Array.isArray(files) ? files : [];
  }
  return Object.keys(coverage);
}

function missingSourceMessage(missing, repositoryRoot) {
  const listed = missing.slice(0, MISSING_LISTED).join(', ');
  const rest = missing.length > MISSING_LISTED ? ' (+' + (missing.length - MISSING_LISTED) + ' more)' : '';
  return 'coverage tiers: the native coverage report is missing ' + missing.length + ' adopted source file(s) the policy requires with a threshold: ' + listed + rest
    + '. An exclusion or an include gap removed them from the report, so their thresholds would never be enforced; remove the exclusion, or declare them infrastructure in coverage-tiers.json and resolve again with "dharness policy --root ' + repositoryRoot + '"';
}

// Shared wire/config object boundary; array and thenable restrictions stay at
// the call sites whose native contracts require them.
function isObject(value) {
  return value !== null && typeof value === 'object';
}

function isThenable(value) {
  return isObject(value) && typeof value.then === 'function';
}

function describe(value) {
  if (value === null) {
    return 'null';
  }
  if (Array.isArray(value)) {
    return 'a list';
  }
  return typeof value;
}
