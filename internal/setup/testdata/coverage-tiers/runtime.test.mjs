import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const [scratch, moduleFile, policyBin, faultBin] = process.argv.slice(2);
const { withCoverageTiers } = await import(pathToFileURL(moduleFile));
const originalPath = process.env.PATH ?? process.env.Path ?? '';
function policyPath(value) {
  for (const key of Object.keys(process.env)) {
    if (key.toUpperCase() === 'PATH') delete process.env[key];
  }
  process.env.PATH = value;
}
policyPath(policyBin + path.delimiter + originalPath);

function write(root, file, text) {
  const target = path.join(root, file);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, text);
}

// Real Go policy resolution over source files, not a second tier classifier.
// `extra` widens the adopted universe (core tier) so the number of enforced
// sources stops being three, and `infrastructureOnly` empties it of anything
// that carries a threshold.
function consumer(name, { split = false, provider = true, extra = [], infrastructureOnly = false } = {}) {
  const root = path.join(scratch, name);
  const project = split ? path.join(root, 'frontend') : root;
  const prefix = split ? 'frontend/src/cart/' : 'src/cart/';
  write(root, 'coverage-tiers.json', JSON.stringify({
    version: 1, enforcedPrefixes: [prefix], features: [{
      id: 'cart', path: prefix, defaultTier: infrastructureOnly ? 'infrastructure' : 'core', reason: 'Cart behavior',
      overrides: infrastructureOnly ? [] : [
        { path: prefix + 'ui.js', tier: 'important', reason: 'Presentation' },
        { path: prefix + 'types.js', tier: 'infrastructure', reason: 'Wiring' },
      ],
    }],
  }));
  for (const file of ['old.js', 'query.js', 'ui.js', 'types.js', ...extra]) {
    write(root, prefix + file, 'export const value = 1;\n');
  }
  write(project, 'package.json', '{"private":true,"type":"module"}');
  // Fixed native API envelope only: no imitation of Vitest's flag grammar.
  write(project, 'node_modules/vitest/package.json', '{"exports":{"./node":"./node.cjs"}}');
  write(project, 'node_modules/vitest/node.cjs', `exports.parseCLI = (argv) => {
    globalThis.seenCLI = [...argv];
    argv.shift();
    return globalThis.nativeCLI;
  };`);
  if (provider) {
    write(project, 'node_modules/@vitest/coverage-v8/package.json', '{"main":"index.cjs"}');
    write(project, 'node_modules/@vitest/coverage-v8/index.cjs', 'module.exports = {};');
  }
  return { root, project, prefix, options: { repositoryRoot: root, sourceRoot: split ? 'frontend' : '.' } };
}

function snapshot(root) {
  return fs.readdirSync(root, { recursive: true }).filter((name) => fs.statSync(path.join(root, name)).isFile())
    .sort().map((name) => [name, fs.readFileSync(path.join(root, name), 'utf8')]);
}

async function cli(argv, coverage, run) {
  return nativeCli(argv, { filter: [], options: coverage === undefined ? {} : { coverage } }, run);
}

// The same complete command line with the runner's own answer spelled out, so a
// parse result that is not the {filter, options} envelope is reachable too.
async function nativeCli(argv, native, run) {
  const original = process.argv;
  process.argv = ['node', 'vitest', ...argv];
  globalThis.nativeCLI = native;
  try {
    return await run();
  } finally {
    process.argv = original;
  }
}

const enabled = () => ({ test: { coverage: { enabled: true } } });
const target = (value) => ({ statements: value, branches: value, functions: value, lines: value, perFile: true });

// S1/S2/S4/S5: the same complete adopted unit, including old source, in both layouts.
for (const split of [false, true]) {
  const c = consumer(split ? 'split' : 'root', { split });
  const base = enabled();
  const before = structuredClone(base);
  const filesBefore = snapshot(c.root);
  const result = withCoverageTiers(base, c.options);
  assert.deepEqual(result.test.coverage.include, [
    'src/cart/old.js', 'src/cart/query.js', 'src/cart/types.js', 'src/cart/ui.js',
  ]);
  assert.deepEqual(result.test.coverage.thresholds, {
    'src/cart/old.js': target(100), 'src/cart/query.js': target(100),
    'src/cart/types.js': target(0), 'src/cart/ui.js': target(80),
  });
  assert.deepEqual(base, before);
  assert.deepEqual(snapshot(c.root), filesBefore);
  assert.equal(result.test.reporters[0], 'default');

  // S1/S5: the projection depends on the config's shape, not on how it arrived.
  // A root project is the default sourceRoot, and a callable whose value is
  // still pending gets projected once it settles — swapped for the bare
  // promise it would leave the consumer with an unenforced config.
  if (!split) {
    assert.deepEqual(withCoverageTiers(enabled(), { repositoryRoot: c.root }), result);
  }
  const callable = withCoverageTiers(() => enabled(), c.options);
  assert.deepEqual(callable(), result);
  const pending = withCoverageTiers(() => Promise.resolve(enabled()), c.options);
  assert.ok(pending() instanceof Promise);
  const settled = await pending();
  assert.deepEqual(settled.test.coverage.thresholds, result.test.coverage.thresholds);
  assert.deepEqual(settled.test.coverage.include, result.test.coverage.include);

  // S3/S4: presence is required even if an exclusion hid old adopted code.
  const reporter = result.test.reporters.at(-1);
  const covered = ['old.js', 'query.js', 'ui.js'].map((file) => path.join(c.root, c.prefix, file));
  assert.doesNotThrow(() => reporter.onCoverage({ files: () => covered })); // INFRA0 may be absent.
  // The provider's report has two accepted shapes, and an entry that is not a
  // path is skipped rather than resolved into one.
  assert.doesNotThrow(() => reporter.onCoverage(Object.fromEntries(covered.map((file) => [file, {}]))));
  assert.doesNotThrow(() => reporter.onCoverage({ files: () => [...covered, null, 42] }));
  assert.throws(() => reporter.onCoverage(null), {
    message: `coverage tiers: the native coverage report is missing 3 adopted source file(s) the policy requires with a threshold: ${c.prefix}old.js, ${c.prefix}query.js, ${c.prefix}ui.js. An exclusion or an include gap removed them from the report, so their thresholds would never be enforced; remove the exclusion, or declare them infrastructure in coverage-tiers.json and resolve again with "dharness policy --root ${c.root}"`,
  });
  for (const missing of ['old.js', 'ui.js']) { // Both CORE and IMPORTANT are required.
    const measured = covered.filter(file => file !== path.join(c.root, c.prefix, missing));
    assert.throws(() => reporter.onCoverage({ files: () => measured }), {
      message: `coverage tiers: the native coverage report is missing 1 adopted source file(s) the policy requires with a threshold: ${c.prefix}${missing}. An exclusion or an include gap removed them from the report, so their thresholds would never be enforced; remove the exclusion, or declare them infrastructure in coverage-tiers.json and resolve again with "dharness policy --root ${c.root}"`,
    });
  }
  assert.deepEqual(snapshot(c.root), filesBefore);
}

// S1/S4: a project root that does not contain an adopted source is a refusal
// naming the file, not a silent projection of a path the runner cannot read.
const outside = consumer('outside-project-root', { split: true });
write(outside.root, 'shared/lib/util.js', 'export const value = 1;\n');
write(outside.root, 'coverage-tiers.json', JSON.stringify({
  version: 1, enforcedPrefixes: [outside.prefix, 'shared/lib/'], features: [
    { id: 'cart', path: outside.prefix, defaultTier: 'core', reason: 'Cart behavior', overrides: [] },
    { id: 'shared', path: 'shared/lib/', defaultTier: 'core', reason: 'Shared behavior', overrides: [] },
  ],
}));
const outsideFiles = snapshot(outside.root);
const outsideConfig = enabled();
assert.throws(() => withCoverageTiers(outsideConfig, outside.options), {
  message: `coverage tiers: adopted source shared/lib/util.js is outside this project root (${outside.project}); a test project only measures its own root, so point sourceRoot at the shared root of the adopted prefixes or narrow the prefix this project owns — declaring the tiers and not enforcing them here is not a silent option`,
});
assert.deepEqual(outsideConfig, enabled());
assert.deepEqual(snapshot(outside.root), outsideFiles);

// S2/S5: ordinary/focused runs need no policy or provider; native CLI false wins.
const ordinary = consumer('ordinary', { provider: false });
write(ordinary.root, 'coverage-tiers.json', '{"version":2}');
const ordinaryFiles = snapshot(ordinary.root);
policyPath(''); // Resolving a policy accidentally must fail, not return an inactive answer.
await cli(['run', 'cart.test.js'], undefined, async () => {
  const base = { test: { coverage: { enabled: false }, reporters: ['verbose'] } };
  const before = structuredClone(base);
  assert.equal(withCoverageTiers(base, ordinary.options), base);
  const sync = withCoverageTiers((env) => { assert.equal(env.mode, 'test'); return base; }, ordinary.options);
  assert.equal(sync({ mode: 'test' }), base);
  const asyncConfig = withCoverageTiers(async () => base, ordinary.options);
  assert.equal(await asyncConfig(), base);
  assert.deepEqual(base, before);
});
for (const argv of [['run', '--coverage=false'], ['run', '--coverage.enabled', 'false']]) {
  await cli(argv, { enabled: false }, () => {
    const base = enabled();
    const before = structuredClone(base);
    assert.equal(withCoverageTiers(base, ordinary.options), base);
    assert.deepEqual(base, before);
    assert.deepEqual(globalThis.seenCLI, ['vitest', ...argv]);
    assert.deepEqual(process.argv, ['node', 'vitest', ...argv]);
  });
}
assert.deepEqual(snapshot(ordinary.root), ordinaryFiles);
policyPath(policyBin + path.delimiter + originalPath);

// S2/S5: the runner's own answer is what decides. A mention that says nothing
// about `enabled` leaves the config's value in force, an explicit value on
// either side wins, and a config that states nothing is never read as enabled.
const precedence = consumer('cli-precedence');
const precedenceFiles = snapshot(precedence.root);
const adopted = ['src/cart/old.js', 'src/cart/query.js', 'src/cart/types.js', 'src/cart/ui.js'];
function projectedConfig(config = enabled()) {
  const result = withCoverageTiers(config, precedence.options);
  assert.notEqual(result, config);
  assert.deepEqual(result.test.coverage.include, adopted);
  assert.equal(result.test.reporters.length, 2);
  return result;
}
function untouchedConfig(config = enabled()) {
  const before = structuredClone(config);
  assert.equal(withCoverageTiers(config, precedence.options), config);
  assert.deepEqual(config, before);
  return config;
}
await nativeCli(['run', '--coverage'], { filter: [], options: { coverage: {} } }, () => projectedConfig());
await nativeCli(['run', '--coverage'], { filter: [], options: { coverage: { enabled: false } } }, () => untouchedConfig());
await nativeCli(['run', '--coverage'], { filter: [], options: { coverage: { enabled: true } } }, () => projectedConfig({ test: { coverage: { enabled: false } } }));
await nativeCli(['run', '--coverage'], { filter: [], options: { coverage: {} } }, () => untouchedConfig({ test: { coverage: { enabled: false } } }));
for (const parsed of [null, {}, { filter: [] }, { filter: [], options: {} }, { filter: [], options: { coverage: 'yes' } }]) {
  await nativeCli(['run', '--coverage.enabled', 'true'], parsed, () => projectedConfig());
  await nativeCli(['run', '--coverage.enabled', 'true'], parsed, () => {
    const config = { test: { coverage: { enabled: false } } };
    assert.equal(withCoverageTiers(config, precedence.options), config);
  });
}
for (const silent of [{}, { test: {} }, { test: { coverage: null } }]) {
  assert.equal(withCoverageTiers(silent, precedence.options), silent);
}
assert.deepEqual(snapshot(precedence.root), precedenceFiles);
const unadopted = consumer('unadopted', { provider: false });
fs.unlinkSync(path.join(unadopted.root, 'coverage-tiers.json'));
const unadoptedFiles = snapshot(unadopted.root);
const unadoptedConfig = enabled();
const unadoptedBefore = structuredClone(unadoptedConfig);
assert.equal(withCoverageTiers(unadoptedConfig, unadopted.options), unadoptedConfig);
assert.deepEqual(unadoptedConfig, unadoptedBefore);
assert.deepEqual(snapshot(unadopted.root), unadoptedFiles);

// S1/S5: consumer decisions survive; a weaker adopted threshold cannot overwrite either side.
const preserved = consumer('preservation');
const base = { resolve: { alias: { cart: './src/cart' } }, test: {
  reporters: ['verbose'], coverage: {
    enabled: true, include: ['src/legacy.js'], exclude: ['**/generated/**'],
    thresholds: { 'src/legacy.js': { lines: 17 }, 'src/cart/ui.js': target(95) },
  },
} };
const before = structuredClone(base);
const preservedFiles = snapshot(preserved.root);
const projected = withCoverageTiers(base, preserved.options);
assert.deepEqual(projected.resolve, base.resolve);
assert.deepEqual(projected.test.coverage.exclude, ['**/generated/**']);
assert.equal(projected.test.coverage.include[0], 'src/legacy.js');
assert.deepEqual(projected.test.coverage.thresholds['src/legacy.js'], { lines: 17 });
assert.deepEqual(projected.test.coverage.thresholds['src/cart/ui.js'], target(95));
assert.equal(projected.test.reporters[0], 'verbose');
assert.equal(projected.test.reporters.length, 2);
assert.deepEqual(base, before);
const weak = structuredClone(base);
weak.test.coverage.thresholds['src/cart/ui.js'] = target(70);
const weakBefore = structuredClone(weak);
assert.throws(() => withCoverageTiers(weak, preserved.options), {
  message: 'coverage tiers: conflict on src/cart/ui.js: the resolved tier requires statements >= 80, and the consumer declared 70; raise the threshold to the declared tier in coverage-tiers.json, or raise the consumer value — dharness never resolves this by overwriting either one',
});
assert.deepEqual(weak, weakBefore);
assert.deepEqual(snapshot(preserved.root), preservedFiles);

// S2/S5: the wrapper's own boundary. Options and config are refused with the
// exact fix named, before any policy is asked for.
const optionsTree = consumer('options-refusal');
const optionsFiles = snapshot(optionsTree.root);
const requiresOptions = 'coverage tiers: withCoverageTiers requires an options object { repositoryRoot, sourceRoot }';
for (const bad of [undefined, null, 'D:/repo', 42, true]) {
  assert.throws(() => withCoverageTiers(enabled(), bad), { message: requiresOptions });
}
const requiresRoot = 'coverage tiers: withCoverageTiers requires repositoryRoot, the directory holding coverage-tiers.json';
for (const repositoryRoot of [undefined, '', '   ', 42, null]) {
  assert.throws(() => withCoverageTiers(enabled(), { repositoryRoot }), { message: requiresRoot });
}
const requiresSource = 'coverage tiers: sourceRoot must be a path relative to repositoryRoot, or "."';
for (const sourceRoot of ['', '  ', 42, null]) {
  assert.throws(() => withCoverageTiers(enabled(), { repositoryRoot: optionsTree.root, sourceRoot }), { message: requiresSource });
}
for (const sourceRoot of ['..', '../outside', '../deep/inside', path.resolve(optionsTree.root, '..')]) {
  assert.throws(() => withCoverageTiers(enabled(), { repositoryRoot: optionsTree.root, sourceRoot }), {
    message: `coverage tiers: sourceRoot ${JSON.stringify(sourceRoot)} escapes repositoryRoot; the project root must be the repository root or a directory inside it`,
  });
}
const requiresConfig = 'coverage tiers: withCoverageTiers accepts a Vitest config object or a config function returning one; pass the result of defineConfig(...)';
for (const bad of [null, [], 'config', 42, Promise.resolve({})]) {
  assert.throws(() => withCoverageTiers(bad, optionsTree.options), { message: requiresConfig });
}
for (const bad of [() => [], () => ['nested']]) {
  assert.throws(() => withCoverageTiers(bad, optionsTree.options)({ mode: 'test' }), { message: requiresConfig });
}
await assert.rejects(withCoverageTiers(() => Promise.resolve([]), optionsTree.options)(), { message: requiresConfig });
assert.deepEqual(snapshot(optionsTree.root), optionsFiles);

// S1/S5: a consumer setting the wrapper cannot merge into is a refusal that
// quotes the coordinate, and a setting it can merge into is preserved once.
const shapes = consumer('consumer-shapes');
const shapesFiles = snapshot(shapes.root);
const needsList = 'coverage tiers: test.coverage.include must be a list of globs for the adopted sources to be added to it; found ';
for (const [include, found] of [[null, 'null'], [{}, 'object'], [42, 'number']]) {
  assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, include } } }, shapes.options), { message: needsList + found });
}
const needsObject = 'coverage tiers: test.coverage.thresholds must be an object so resolved per-file tiers can merge into it; found ';
for (const [thresholds, found] of [[[], 'a list'], ['x', 'string'], [null, 'null'], [7, 'number']]) {
  assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, thresholds } } }, shapes.options), { message: needsObject + found });
}
const needsEntry = 'coverage tiers: test.coverage.thresholds["src/cart/ui.js"] must be an object to merge the resolved tier into; found ';
for (const [entry, found] of [[80, 'number'], [[], 'a list'], [null, 'null']]) {
  assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, thresholds: { 'src/cart/ui.js': entry } } } }, shapes.options), { message: needsEntry + found });
}
const needsNumber = 'coverage tiers: test.coverage.thresholds["src/cart/ui.js"].lines must be a number; found ';
for (const [declared, found] of [['17', 'string'], [NaN, 'number'], [Infinity, 'number'], [null, 'null']]) {
  assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, thresholds: { 'src/cart/ui.js': { lines: declared } } } } }, shapes.options), { message: needsNumber + found });
}
assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, thresholds: { 'src/cart/ui.js': { perFile: false } } } } }, shapes.options), {
  message: 'coverage tiers: test.coverage.thresholds["src/cart/ui.js"].perFile is false, and an adopted source is enforced per file; remove the entry and let the resolved tier declare it, or change the classification in coverage-tiers.json',
});
const needsReporters = 'coverage tiers: test.reporters must be a reporter name or a list of reporters so the adopted-source check can be appended; found ';
for (const [reporters, found] of [[42, 'number'], [{}, 'object'], [null, 'null']]) {
  assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true }, reporters } }, shapes.options), { message: needsReporters + found });
}
const stringInclude = withCoverageTiers({ test: { coverage: { enabled: true, include: 'src/**/*.js' } } }, shapes.options);
assert.deepEqual(stringInclude.test.coverage.include, ['src/**/*.js', 'src/cart/old.js', 'src/cart/query.js', 'src/cart/types.js', 'src/cart/ui.js']);
const deduped = withCoverageTiers({ test: { coverage: { enabled: true, include: ['src/cart/ui.js', 'src/legacy.js'] } } }, shapes.options);
assert.deepEqual(deduped.test.coverage.include, ['src/cart/ui.js', 'src/legacy.js', 'src/cart/old.js', 'src/cart/query.js', 'src/cart/types.js']);
const equalsTier = withCoverageTiers({ test: { coverage: { enabled: true, thresholds: { 'src/cart/ui.js': target(80) } } } }, shapes.options);
assert.deepEqual(equalsTier.test.coverage.thresholds['src/cart/ui.js'], target(80));
const stringReporters = withCoverageTiers({ test: { coverage: { enabled: true }, reporters: 'verbose' } }, shapes.options);
assert.equal(stringReporters.test.reporters[0], 'verbose');
assert.equal(stringReporters.test.reporters.length, 2);
assert.deepEqual(snapshot(shapes.root), shapesFiles);

// S3/S4: an adopted universe that carries no threshold adds no reporter, and a
// declaration that adopts nothing keeps the consumer's config as written.
const infrastructure = consumer('infrastructure-only', { infrastructureOnly: true });
const infrastructureFiles = snapshot(infrastructure.root);
const infrastructureResult = withCoverageTiers(enabled(), infrastructure.options);
assert.equal('reporters' in infrastructureResult.test, false);
assert.deepEqual(infrastructureResult.test.coverage.thresholds, {
  'src/cart/old.js': target(0), 'src/cart/query.js': target(0),
  'src/cart/types.js': target(0), 'src/cart/ui.js': target(0),
});
assert.deepEqual(snapshot(infrastructure.root), infrastructureFiles);
const empty = consumer('empty-universe');
fs.rmSync(path.join(empty.root, empty.prefix), { recursive: true });
fs.mkdirSync(path.join(empty.root, empty.prefix), { recursive: true });
const emptyFiles = snapshot(empty.root);
const emptyConfig = enabled();
assert.equal(withCoverageTiers(emptyConfig, empty.options), emptyConfig);
assert.deepEqual(emptyConfig, enabled());
assert.deepEqual(snapshot(empty.root), emptyFiles);

// S2/S3/S5: the provider is the consumer's decision — its package is named
// when missing, an unknown name is left to the runner, and a provider instance
// is not a package name however it stringifies.
const choice = consumer('provider-choice', { provider: false });
const choiceFiles = snapshot(choice.root);
assert.throws(() => withCoverageTiers({ test: { coverage: { enabled: true, provider: 'istanbul' } } }, choice.options), {
  message: `coverage tiers: the "istanbul" coverage provider needs "@vitest/coverage-istanbul", which is not installed in ${choice.project}; install it locally before running coverage (for example: bun add -d @vitest/coverage-istanbul) — dharness never downloads it at run time`,
});
const unknownProvider = withCoverageTiers({ test: { coverage: { enabled: true, provider: 'coverage-something' } } }, choice.options);
assert.equal(unknownProvider.test.coverage.provider, 'coverage-something');
const instanceProvider = { name: 'istanbul', toString: () => 'istanbul' };
const instanceResult = withCoverageTiers({ test: { coverage: { enabled: true, provider: instanceProvider } } }, choice.options);
assert.equal(instanceResult.test.coverage.provider, instanceProvider);
assert.deepEqual(snapshot(choice.root), choiceFiles);

// S2/S5: every way the policy process itself can fail is a refusal that names
// the command the consumer has to run, never "no adoption".
const failing = consumer('policy-process');
const failingFiles = snapshot(failing.root);
await cli(['run', '--coverage'], { enabled: true }, () => {
  const failingConfig = enabled();
  const failingBefore = structuredClone(failingConfig);
  policyPath(''); // No executable anywhere on PATH.
  assert.throws(() => withCoverageTiers(failingConfig, failing.options), {
    message: `coverage tiers: "dharness" is not on the PATH, so the coverage tiers cannot be resolved; install dharness and run "dharness policy --root ${failing.root}" from the repository`,
  });
  assert.deepEqual(failingConfig, failingBefore);
  policyPath(policyBin + path.delimiter + originalPath);
  // A shell-only or corrupt entry is the Windows shape of "not startable":
  // the runtime accepts a candidate it cannot launch. On POSIX the loader skips
  // an unexecutable candidate and finds the real binary behind it, so there is
  // no equivalent shape to reproduce there.
  if (process.platform === 'win32') {
    const shim = path.join(scratch, 'shim-bin');
    fs.mkdirSync(shim, { recursive: true });
    fs.writeFileSync(path.join(shim, 'dharness'), 'not an executable');
    fs.writeFileSync(path.join(shim, 'dharness.exe'), 'not an executable');
    policyPath(shim + path.delimiter + policyBin + path.delimiter + originalPath);
    let startCode;
    try {
      execFileSync('dharness', ['policy', '--root', failing.root], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
    } catch (error) {
      assert.notEqual(typeof error.status, 'number');
      startCode = error.code;
    }
    assert.equal(typeof startCode, 'string');
    assert.throws(() => withCoverageTiers(failingConfig, failing.options), {
      message: `coverage tiers: "dharness policy --root ${failing.root}" could not be started (${startCode}); dharness must be an executable the runtime can start directly, not a shell-only shim`,
    });
    assert.deepEqual(failingConfig, failingBefore);
    policyPath(policyBin + path.delimiter + originalPath);
  }
});
assert.deepEqual(snapshot(failing.root), failingFiles);

// S2/S5: the resolver's own failures keep their state on the wire.
const wireBefore = process.env.__DHARNESS_TEST_POLICY_WIRE;
const exitBefore = process.env.__DHARNESS_TEST_POLICY_EXIT;
policyPath(faultBin + path.delimiter + originalPath);
try {
  await cli(['run', '--coverage'], { enabled: true }, () => {
    const badJSON = '{not json';
    process.env.__DHARNESS_TEST_POLICY_WIRE = badJSON;
    let parseDetail;
    try {
      JSON.parse(badJSON);
    } catch (error) {
      parseDetail = error.message;
    }
    assert.throws(() => withCoverageTiers(enabled(), failing.options), {
      message: `coverage tiers: "dharness policy --root ${failing.root}" did not return JSON: ${parseDetail}`,
    });
    // A resolver that dies silently is reported with its exit status and
    // Node's own start-failure text, never a bare colon.
    const silentExit = '3';
    process.env.__DHARNESS_TEST_POLICY_WIRE = '';
    process.env.__DHARNESS_TEST_POLICY_EXIT = silentExit;
    const command = ['policy', '--root', failing.root];
    let silentDetail;
    try {
      execFileSync('dharness', command, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
    } catch (error) {
      assert.equal(error.status, Number(silentExit));
      assert.equal(error.stderr, '');
      silentDetail = String(error.message);
    }
    assert.equal(typeof silentDetail, 'string');
    assert.throws(() => withCoverageTiers(enabled(), failing.options), {
      message: `coverage tiers: "dharness policy --root ${failing.root}" failed (exit ${silentExit}): ${silentDetail}`,
    });
    delete process.env.__DHARNESS_TEST_POLICY_EXIT;
    process.env.__DHARNESS_TEST_POLICY_WIRE = JSON.stringify({ version: 1, active: false, files: [] });
    const inactive = enabled();
    assert.equal(withCoverageTiers(inactive, failing.options), inactive);
  });
} finally {
  policyPath(policyBin + path.delimiter + originalPath);
  if (wireBefore === undefined) delete process.env.__DHARNESS_TEST_POLICY_WIRE;
  else process.env.__DHARNESS_TEST_POLICY_WIRE = wireBefore;
  if (exitBefore === undefined) delete process.env.__DHARNESS_TEST_POLICY_EXIT;
  else process.env.__DHARNESS_TEST_POLICY_EXIT = exitBefore;
}
assert.deepEqual(snapshot(failing.root), failingFiles);

// S2/S5: the command line is the runner's to read, and a runner that cannot be
// asked is named with its fix instead of being replaced by a second grammar.
for (const broken of ['absent', 'no-parsecli']) {
  const vitest = consumer('vitest-' + broken);
  if (broken === 'absent') {
    fs.rmSync(path.join(vitest.project, 'node_modules/vitest'), { recursive: true });
  } else {
    write(vitest.project, 'node_modules/vitest/node.cjs', 'module.exports = {};\n');
  }
  const vitestFiles = snapshot(vitest.root);
  let detail = 'resolvable';
  try {
    createRequire(path.join(vitest.project, 'package.json'))('vitest/node');
  } catch (error) {
    detail = error.message;
  }
  // S2: nothing on the command line mentions coverage, so the config's own
  // `enabled` decides and the runner is never asked for its flags. A plain
  // focused run therefore still projects the tiers in the same project whose
  // `vitest/node` cannot answer the coverage question — the recognition of a
  // coverage mention is the whole reason the runner is not loaded here.
  await nativeCli(['run', 'cart.test.js'], { filter: [], options: {} }, () => {
    const plainConfig = enabled();
    const plainBefore = structuredClone(plainConfig);
    const plainProjected = withCoverageTiers(plainConfig, vitest.options);
    assert.notEqual(plainProjected, plainConfig);
    assert.deepEqual(plainProjected.test.coverage.include, adopted);
    assert.deepEqual(plainProjected.test.coverage.thresholds, {
      'src/cart/old.js': target(100), 'src/cart/query.js': target(100),
      'src/cart/types.js': target(0), 'src/cart/ui.js': target(80),
    });
    assert.equal(plainProjected.test.reporters.length, 2);
    assert.deepEqual(plainConfig, plainBefore);
  });
  await cli(['run', '--coverage'], { enabled: true }, () => {
    const vitestConfig = enabled();
    assert.throws(() => withCoverageTiers(vitestConfig, vitest.options), {
      message: broken === 'absent'
        ? `coverage tiers: the installed Vitest API "vitest/node" could not be loaded from ${vitest.project} (${detail}); the coverage command line must be read by Vitest itself, so install vitest where this project can resolve it and run again — dharness never reads the flags on its own and never downloads the runner`
        : `coverage tiers: the installed Vitest at ${vitest.project} does not expose parseCLI from "vitest/node"; this module reads the coverage command line with the runner it integrates with, so upgrade or reinstall vitest in this project instead of letting dharness guess the flags`,
    });
    assert.deepEqual(vitestConfig, enabled());
  });
  assert.deepEqual(snapshot(vitest.root), vitestFiles);
}

// S5: a runner that is installed but fails while loading is reported with the
// runtime's own rendering of that failure. An error carrying no message must
// still be named — "(Error)" — because the refusal exists to say what went
// wrong, and an empty pair of parentheses says nothing.
const throwing = consumer('vitest-throwing');
write(throwing.project, 'node_modules/vitest/node.cjs', 'throw new Error();\n');
const throwingFiles = snapshot(throwing.root);
await cli(['run', '--coverage'], { enabled: true }, () => {
  const throwingConfig = enabled();
  assert.throws(() => withCoverageTiers(throwingConfig, throwing.options), {
    message: `coverage tiers: the installed Vitest API "vitest/node" could not be loaded from ${throwing.project} (Error); the coverage command line must be read by Vitest itself, so install vitest where this project can resolve it and run again — dharness never reads the flags on its own and never downloads the runner`,
  });
  assert.deepEqual(throwingConfig, enabled());
});
assert.deepEqual(snapshot(throwing.root), throwingFiles);

// S3/S4: the refusal lists what it can and counts the rest, and the bound is
// the fifth name, not the fifth-plus-one.
for (const [name, extra, expected] of [
  ['missing-five', ['a.js', 'b.js'], 'src/cart/a.js, src/cart/b.js, src/cart/old.js, src/cart/query.js, src/cart/ui.js'],
  ['missing-many', ['a.js', 'b.js', 'c.js', 'd.js', 'e.js', 'f.js'], 'src/cart/a.js, src/cart/b.js, src/cart/c.js, src/cart/d.js, src/cart/e.js (+4 more)'],
]) {
  const bound = consumer(name, { extra });
  const boundFiles = snapshot(bound.root);
  const boundResult = withCoverageTiers(enabled(), bound.options);
  const boundReporter = boundResult.test.reporters.at(-1);
  assert.throws(() => boundReporter.onCoverage({ files: () => [] }), {
    message: `coverage tiers: the native coverage report is missing ${3 + extra.length} adopted source file(s) the policy requires with a threshold: ${expected}. An exclusion or an include gap removed them from the report, so their thresholds would never be enforced; remove the exclusion, or declare them infrastructure in coverage-tiers.json and resolve again with "dharness policy --root ${bound.root}"`,
  });
  assert.deepEqual(snapshot(bound.root), boundFiles);
}

// S2/S3/S5/S14: an unusable policy answer is a refusal, never "no adoption".
// One scenario, two faces of the same rule: the resolver's own failure and a
// wire reply that does not match the schema each refuse with their own exact
// wording, and the consumer's config and tree are untouched either way.
// The fault executable emits malformed wire data only; all valid policies above
// still use the actual Go resolver over consumer source files.
const unusable = consumer('unusable-policy');
write(unusable.root, 'coverage-tiers.json', '{"version":2}');
const unusableFiles = snapshot(unusable.root);
await cli(['run', '--coverage'], { enabled: true }, () => {
  const off = { test: { coverage: { enabled: false } } };
  assert.throws(() => withCoverageTiers(off, unusable.options), {
    message: `coverage tiers: "dharness policy --root ${unusable.root}" failed (exit 1): dharness: coverage-tiers.json: version must be 1`,
  });
  assert.deepEqual(off, { test: { coverage: { enabled: false } } });
});
assert.deepEqual(snapshot(unusable.root), unusableFiles);
const config = enabled();
const configBefore = structuredClone(config);
const previousWire = process.env.__DHARNESS_TEST_POLICY_WIRE;
policyPath(faultBin + path.delimiter + originalPath);
try {
  const schemaMessage = `coverage tiers: "dharness policy --root ${unusable.root}" returned an unexpected schema; expected {"version":1,"active":boolean,"files":[{"path","tier","coverage"}]}`;
  const entryMessage = `coverage tiers: "dharness policy --root ${unusable.root}" returned a file entry without {path, tier, coverage}`;
  for (const [wire, message] of [
    [JSON.stringify({ version: 1, files: [] }), schemaMessage],
    [JSON.stringify({ version: 1, active: null, files: [] }), schemaMessage],
    [JSON.stringify({ version: 1, active: 'true', files: [] }), schemaMessage],
    [JSON.stringify({ version: 2, active: false, files: [] }), schemaMessage],
    [JSON.stringify({ version: 1, active: false, files: {} }), schemaMessage],
    ['null', schemaMessage],
    [JSON.stringify({ version: 1, active: false, files: [null] }), entryMessage],
    [JSON.stringify({ version: 1, active: false, files: [{ path: '', tier: 'core', coverage: 100 }] }), entryMessage],
    [JSON.stringify({ version: 1, active: false, files: [{ path: 7, tier: 'core', coverage: 100 }] }), entryMessage],
    [JSON.stringify({ version: 1, active: false, files: [{ path: 'src/cart/ui.js', tier: 7, coverage: 100 }] }), entryMessage],
    [JSON.stringify({ version: 1, active: false, files: [{ path: 'src/cart/ui.js', tier: 'core' }] }), entryMessage],
    [JSON.stringify({ version: 1, active: false, files: [{ path: 'src/cart/ui.js', tier: 'core', coverage: '100' }] }), entryMessage],
    ['{"version":1,"active":false,"files":[{"path":"src/cart/ui.js","tier":"core","coverage":1e999}]}', entryMessage],
  ]) {
    process.env.__DHARNESS_TEST_POLICY_WIRE = wire;
    assert.throws(() => withCoverageTiers(config, unusable.options), { message });
    assert.deepEqual(config, configBefore);
    assert.deepEqual(snapshot(unusable.root), unusableFiles);
  }
  // A well-formed answer is projected: the schema guard refuses what it cannot
  // read without refusing what it can.
  process.env.__DHARNESS_TEST_POLICY_WIRE = JSON.stringify({
    version: 1, active: true,
    files: [{ path: 'src/cart/ui.js', tier: 'important', coverage: 80 }],
  });
  const accepted = enabled();
  const projectedWire = withCoverageTiers(accepted, unusable.options);
  assert.deepEqual(projectedWire.test.coverage.include, ['src/cart/ui.js']);
  assert.deepEqual(projectedWire.test.coverage.thresholds, { 'src/cart/ui.js': target(80) });
  assert.equal(projectedWire.test.reporters.length, 2);
  assert.deepEqual(accepted, enabled());
  assert.deepEqual(snapshot(unusable.root), unusableFiles);
} finally {
  policyPath(policyBin + path.delimiter + originalPath);
  if (previousWire === undefined) delete process.env.__DHARNESS_TEST_POLICY_WIRE;
  else process.env.__DHARNESS_TEST_POLICY_WIRE = previousWire;
}

// S1/S4: an adopted source the resolver reports cannot be projected onto a path
// this project can measure — the project root itself in a root layout, or its
// parent in a split one — is refused naming the file, never projected. The
// schema guard accepts this answer ('.' is a non-empty repository-relative
// string), so this projection guard is the only thing standing between it and a
// threshold the runner reads as its own project directory. The policy emits
// concrete file paths, so an answer like this comes from a resolver that does
// not, and refusing it is what keeps "declared but not enforced" from being
// silent.
for (const [name, split] of [['misplaced-root', false], ['misplaced-split', true]]) {
  const misplaced = consumer(name, { split });
  const misplacedFiles = snapshot(misplaced.root);
  const previousMisplacedWire = process.env.__DHARNESS_TEST_POLICY_WIRE;
  policyPath(faultBin + path.delimiter + originalPath);
  try {
    process.env.__DHARNESS_TEST_POLICY_WIRE = JSON.stringify({
      version: 1, active: true,
      files: [{ path: '.', tier: 'core', coverage: 100 }],
    });
    await cli(['run', '--coverage'], { enabled: true }, () => {
      const misplacedConfig = enabled();
      assert.throws(() => withCoverageTiers(misplacedConfig, misplaced.options), {
        message: `coverage tiers: adopted source . is outside this project root (${misplaced.project}); a test project only measures its own root, so point sourceRoot at the shared root of the adopted prefixes or narrow the prefix this project owns — declaring the tiers and not enforcing them here is not a silent option`,
      });
      assert.deepEqual(misplacedConfig, enabled());
    });
  } finally {
    policyPath(policyBin + path.delimiter + originalPath);
    if (previousMisplacedWire === undefined) delete process.env.__DHARNESS_TEST_POLICY_WIRE;
    else process.env.__DHARNESS_TEST_POLICY_WIRE = previousMisplacedWire;
  }
  assert.deepEqual(snapshot(misplaced.root), misplacedFiles);
}

const missingProvider = consumer('missing-provider', { provider: false });
const providerFiles = snapshot(missingProvider.root);
const requested = enabled();
assert.throws(() => withCoverageTiers(requested, missingProvider.options), {
  message: `coverage tiers: the "v8" coverage provider needs "@vitest/coverage-v8", which is not installed in ${missingProvider.project}; install it locally before running coverage (for example: bun add -d @vitest/coverage-v8) — dharness never downloads it at run time`,
});
assert.deepEqual(requested, enabled());
assert.deepEqual(snapshot(missingProvider.root), providerFiles);
process.stdout.write('coverage tiers runtime contracts passed\n');
