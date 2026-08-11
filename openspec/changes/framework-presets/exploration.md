# Exploration: framework-presets

> Materialised by the orchestrator: the exploring agent had no write access, as
> in the previous change. The body is its report. The section marked
> **Orchestrator correction** is not — it overturns one of its conclusions, and
> is separated so the disagreement stays visible rather than being blended away.

## Current State

`internal/project/detect.go` builds one `Project{Root, Source, PackageManager,
TestRunner, YarnPnP, InRepository}` from the lockfile and `package.json`.
`Discover` (`internal/project/discover.go`) splits Root (git) from Source (where
the package manager installs), using `git ls-files` against lockfile pathspecs —
precedent for "ask the tool, don't walk the tree". No framework or archetype
field exists anywhere; detection stops at package manager and test runner.

`internal/setup/setup.go` defines `Step{ID, Describe, Satisfied, Delegated,
Apply}` and `Plan()` (11 steps). `agentSkillStep`, `architectureStep`,
`boundariesOwnerStep` and `legacyLintConfigStep` are unconditionally delegated;
`fallowExtendsStep`, `lefthookExtendsStep` and `hookInstallStep` are
conditionally delegated. `internal/setup/plugin.go` holds one global
`DefaultThresholds()` and one global `offByDefault = {"folder-ownership":
true}` — single fixed answers, not per-project.

`ownedFilesStep.Apply` (`steps.go:117`) writes `.dharness/fallow.jsonc` as a
**permanently empty** commented JSONC object. dharness owns that file outright
already. That is the concrete gap behind the `wailsjs/**` example: not a missing
step, but a step dharness already runs unconditionally that writes nothing
framework-specific.

## Affected Areas

- `internal/project/detect.go` — where `detectFramework(...)` would join
  `detectPackageManager` and `detectTestRunner`.
- `internal/project/discover.go` — Wails needs a Root-scope signal
  (`wails.json`), distinct from the Source-scope `package.json` dependency
  signal Next.js and Expo use. The existing Root/Source split is the seam.
- `internal/setup/steps.go`, `ownedFilesStep.Apply` (117–139) — the file dharness
  owns outright, and so the only place preset content can land at all.
- `internal/setup/plugin.go` (`DefaultThresholds`, `offByDefault`,
  `DefaultSeverity`) — global constants today; would need per-project
  parameterisation.
- `internal/setup/steps.go`, `architectureStep.Describe` / `ArchitecturePrompt` —
  a preset could seed this Intención prompt without changing that the step stays
  delegated (§21).
- No golden test exists over `Plan()` or `Step` output today. Checked
  `setup_test.go` and `steps_root_test.go`; neither snapshots the full plan.

## Approaches

1. **Per-framework Go package plus registry, adapting the reference `Adapter`
   shape.** One package per framework (`wails`, `expo`, `nextjs`, `generic`);
   Identity, `Detect(p) (matched, evidence)`, and a versioned `Manifest` of
   facts-with-evidence and decisions; one switch in `factory.go`.
   - *For:* mirrors a proven pattern carrying sixteen adapters; the versioned
     manifest with per-fact evidence answers the rot concern directly; keeps
     framework knowledge auditable one file per concern, the way
     `internal/tool` already isolates CLI invocation knowledge.
   - *Against:* needs a composition layer the reference does not have. Its
     agents are mutually exclusive; frameworks are not — a Wails root with a
     Next.js source is explicitly in scope. Composition by scope is new design
     work, not a port.
   - *Effort:* medium.
2. **One table keyed by framework ID, no per-framework files.**
   - *For:* less code for four presets.
   - *Against:* collapses per-fact evidence into one undifferentiated struct,
     defeating the audit property the pattern exists for. Does not scale past
     four, and four is not the ceiling.
   - *Effort:* low, and pays down badly fast.
3. **Push the knowledge upstream into fallow or react-doctor instead.**
   - *Against:* not actionable — no existing command or flag answers this.
     Recorded only because the first rule requires checking before designing.

## Recommendation

Approach 1, with two corrections to the brief surfaced rather than resolved.

**The "fewer delegated steps" framing does not survive contact.** Walking all
seven currently-delegated steps against what a preset could answer: none of them
flips to dharness-executed. `agentSkillStep` stays delegated (Conducción,
unrelated to framework). `architectureStep` stays delegated (§21 — a preset
seeds the prompt, it does not decide zones). `boundariesOwnerStep` and
`legacyLintConfigStep` stay delegated: they detect conflict and breakage in the
*project's own* files, which is orthogonal to framework. The three
`extends`/`hookInstall` steps stay delegated under their existing preconditions —
merging into someone else's file, or a hook-manager choice nobody has made — and
none of those preconditions is a framework fact.

The actual win is a new dimension of value inside `ownedFilesStep`, which
dharness already runs unconditionally and currently writes empty. The proposal
should be framed as **the owned file stops being empty**, not as a step count.

## Orchestrator correction

The exploration also concluded that the `extends` replace hazard "resolves for
free" if presets write only into `.dharness/fallow.jsonc`. **That is wrong, and
the counterexample is the project that motivated this change.**

Measured against fallow 3.14.0: `extends` replaces a key rather than merging it.
A parent declaring `ignoreDependencies` is honoured until the child declares its
own, and from then on the parent's value is discarded whole, with no error. The
same applies to `boundaries`, measured the same way.

The reference project's own `.fallowrc.json` already declares
`"ignorePatterns": ["wailsjs/**"]`. If dharness wrote that key into the file it
owns, the project's declaration would replace it entirely and in silence — the
preset would do nothing, in the exact repository the example came from.

Writing into the owned file is **necessary but not sufficient**. Every key a
preset contributes needs the treatment `boundariesOwnerStep` already gives one
key: dharness writes it in its own file *and* reports when the project's own
config declares the same key, because only one of the two is in effect and the
configuration does not say which. That generalises an existing step from one key
to N, and it is design work the proposal has to carry rather than inherit.

## Risks

- **Wails' output-dir configurability is unverified.** Whether Wails exposes a
  key to relocate `wailsjs/` decides whether "the repo overrides the preset
  default" is load-bearing on day one or hypothetical. Open before the proposal
  asserts the default as fact.
- **`offByDefault["folder-ownership"]` does not cleanly become a preset
  decision.** Whether a project publishes through barrels is observable from the
  tree — do `index.ts` barrels exist? — not a framework fact. Route it to
  detection, and record the reclassification explicitly.
- **Multi-preset composition is in scope, not deferrable.** Wails signals sit at
  Root scope, Next.js and Expo at Source scope. The registry must return facts
  from more than one preset by scope, which a mutually-exclusive agent registry
  never has to do.
- **No golden test covers `Plan()` today**, so "generic reproduces current
  behaviour exactly" has no existing pinning mechanism to lean on. One has to be
  built.
- **Monorepos with several frameworks remain open.** `AmbiguousSourceError`
  (`discover.go`) already refuses to guess between multiple JS projects and is a
  plausible precedent — fail closed, name the candidates — but it was not
  designed with presets in mind.

## Ready for Proposal

Yes, with four questions for the proposal phase to resolve:

1. Reframe the goal as "the owned file stops being empty", or justify a
   step-count change explicitly.
2. Reclassify `folder-ownership`'s off-by-default from preset-decided to
   detection-derived.
3. Decide whether multi-framework monorepos are in scope or explicitly deferred.
4. Design how a preset-contributed key survives a project that declares the same
   key — see **Orchestrator correction**. This is a hard constraint, measured,
   not a preference.
