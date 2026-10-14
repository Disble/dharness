package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestMutateHelpExplainsCoverageTierScope(t *testing.T) {
	var out bytes.Buffer
	if err := RunMutate([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "coverage-tiers.json excludes declared infrastructure; unadopted paths stay eligible") {
		t.Fatalf("mutation help omits the policy scope boundary: %s", out.String())
	}
}
