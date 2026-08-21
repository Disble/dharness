#!/usr/bin/env bash
# Proves the local gate's failure path (L5).
#
# A gate nobody has watched fail is indistinguishable from no gate at all, and
# gates fail silently far more often than they fire wrongly. This stages a
# deliberately broken file, runs the hook, asserts it refused, and cleans up.
#
# Run it after changing the hook, and in CI so a hook that stopped enforcing
# cannot reach main quietly.
set -euo pipefail

hook="$(git rev-parse --show-toplevel)/.githooks/pre-commit"
probe="internal/cli/zz_gate_probe.go"
index_probe="internal/cli/zz_gate_index_probe.go"

cleanup() {
	git reset -q -- "$probe" "$index_probe" 2>/dev/null || true
	rm -f "$probe" "$index_probe"
}
trap cleanup EXIT

# Badly formatted on purpose: gofmt is the first check the hook runs, so this
# proves the hook refuses without waiting on vet or the suite.
printf 'package cli\n\nfunc gateProbe()   int {\n  return 1\n}\n' > "$probe"
git add "$probe"

if "$hook" > /dev/null 2>&1; then
	echo "verify-gate: FAILED — the hook accepted a file it should have refused." >&2
	exit 1
fi


# The second probe is the one this repository paid for. The first proves the
# hook refuses a broken file; this proves it refuses a broken *commit* while
# the working tree is clean — which is every commit in a change split into
# work units, and the shape that put ten unformatted commits into main on
# 21 August 2026. Measured against the previous hook on this exact fixture:
# it exited 0.
printf 'package cli\n\nfunc gateIndexProbe()   int {\n\treturn 1\n}\n' > "$index_probe"
git add "$index_probe"
gofmt -w "$index_probe"

if [ -n "$(gofmt -l "$index_probe")" ]; then
	echo "verify-gate: FAILED — the fixture's working tree is not clean, so this proves nothing." >&2
	exit 1
fi

if "$hook" > /dev/null 2>&1; then
	echo "verify-gate: FAILED — the hook accepted a staged file it should have refused; it is reading the working tree, not the index." >&2
	exit 1
fi

echo "verify-gate: the hook refused a broken file, and refused a broken commit behind a clean working tree."
