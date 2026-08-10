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

cleanup() {
	git reset -q -- "$probe" 2>/dev/null || true
	rm -f "$probe"
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

echo "verify-gate: the hook refused a broken file, as it must."
