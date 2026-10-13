# Native Jest coverage tiers

Status: backlog. The first coverage-tier delivery supports native Vitest coverage only; existing Jest mutation and related-test workflows must keep their current behavior.

User decision:
> ok, primero vitest, jest queda para el backlog.

Future scope:
- Consume the same native dharness policy resolution; do not implement a second classifier.
- Preserve consumer-owned Jest configuration and deliberate overrides.
- Apply 100/80/0 over complete adopted units, including existing and unexecuted source.
- Verify Jest's native per-path/global coverage semantics rather than assuming Vitest equivalence.
- Wire behind the project's existing test/gate workflow, without another runner wrapper.
- Prove the native failure path in a disposable consumer, on Windows as well as other supported platforms.

Not included: numeric mutation-score tiers, semantic-equivalence approval or evidence machinery, external skill publication/installers, or bulk adoption of all legacy code.
