# Backlog: forcing `@latest` is also a supply-chain decision

Not scheduled. Recorded on 2026-08-13, while the Stryker install that made
`@latest` transversal was being merged, so the reasoning does not have to be
reconstructed later.

---

## 1. A flag that stops dharness from forcing the newest version

Every remote invocation dharness builds asks the manager for the newest
release — `react-doctor@latest`, `fallow@latest` — and since 2026-08-13 the
Stryker install does too, on every `mutate` run.

The reason is recorded and still holds. `LatestSpec`'s own doc comment names the
case it was written for: npx once resolved react-doctor 0.2.1 out of a cache
while `react-doctor@latest` resolved 0.9.11. §03 made that resolution part of
what dharness owns, and §09's amendment leans on it again — asking the manager
for `@latest` is what replaced a registry query and a version comparison.

**The cost of that is a supply-chain one, and it points the other way.** `@latest`
means the interval between a version being published and dharness running it is
as short as it can be. Most days that is the good property. On the day a package
is compromised — and it happens to popular npm packages, repeatedly — it is the
fastest available path from a malicious publish to code executing inside a
pre-commit hook on a developer's machine, with no human in between.

So: a transversal flag, off by default, that stops dharness from forcing the
newest version.

What it does *not* settle, and these are the entry:

- **What the pinned side resolves to.** ~~For Stryker there is an obvious answer
  now~~ — **the Stryker half was closed on 2026-08-13 and is no longer part of
  this entry.** A real repository showed the cost was not hypothetical:
  installing at `@latest` rewrote its exact `"9.6.1"` into `"^9.6.1"`, so
  `mutate` now adds at `@latest` only what the project does not declare and
  restores the rest, with `--upgrade` as the explicit override (§05).

  What remains open is the harder half, untouched: react-doctor and fallow run
  through a remote executor and dharness records no version anywhere, so a flag
  there would have to invent a place to write one. And unlike `mutate`, those
  two run in the gate on every commit, where using whatever old copy a project
  happens to have installed is its own hazard. Same words, different problem.
- **Whether it is one switch or one per tool.** A repository might want its
  linters pinned and its mutation engine current, or the reverse.
- **Where it lives.** A CLI flag is per-invocation and the gate is invoked by a
  hook, not by a person; an environment variable is invisible in review; a key
  under `.dharness/` is a file dharness owns, which §03 allows but which then
  travels with the repository and has to be maintained.
- **What it gives up.** Whoever sets it takes back the staleness problem
  `@latest` was introduced to solve, including the one measured on 2026-08-13:
  a project sitting on Stryker 8.2.6 while 9.6.1 is current. The flag does not
  make that go away — it moves the decision to the person setting it, which is
  the point, but the message when it is on should say so.

Nothing here is a promise, and the shape above is not a design. It is the
argument, written down while it was fresh.
