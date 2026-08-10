//go:build mutation

package mutation

import (
	"os"
	"strconv"
	"testing"

	"github.com/gtramontina/ooze"
)

const (
	envIgnorePattern = "DHARNESS_MUTATION_IGNORE"
	envTestCommand   = "DHARNESS_MUTATION_TEST_CMD"
	envThreshold     = "DHARNESS_MUTATION_THRESHOLD"
	envRepositoryDir = "DHARNESS_MUTATION_ROOT"
	envScope         = "DHARNESS_MUTATION_SCOPE"
)

func TestStagedMutation(t *testing.T) {
	testCommand := os.Getenv(envTestCommand)
	if testCommand == "" {
		t.Skip("run through go run ./tools/mutationstaged")
	}
	ranges, err := ParseOffsetRanges(os.Getenv(envScope))
	if err != nil {
		t.Fatalf("%s is malformed: %v", envScope, err)
	}

	options := []ooze.Option{
		ooze.WithRepositoryRoot(os.Getenv(envRepositoryDir)),
		ooze.WithTestCommand(testCommand),
		ooze.WithMinimumThreshold(thresholdFromEnv(t)),
	}
	if ignore := os.Getenv(envIgnorePattern); ignore != "" {
		options = append(options, ooze.IgnoreSourceFiles(ignore))
	}
	if len(ranges) > 0 {
		counter := &ScopeCounter{}
		scoped := ScopeAll(DefaultViruses(), ranges, counter)
		options = append(options, ooze.WithViruses(scoped[0], scoped[1:]...))
	}

	// Zero candidates were rejected by the wrapper's reachable preflight before
	// this call. A guard after Release cannot provide that guarantee: ooze can
	// call t.Fatal internally and make following statements unreachable.
	ooze.Release(t, options...)
}

func thresholdFromEnv(t *testing.T) float32 {
	t.Helper()
	raw := os.Getenv(envThreshold)
	parsed, err := strconv.ParseFloat(raw, 32)
	if err != nil || parsed < 0 || parsed > 1 {
		t.Fatalf("%s=%q must be a number from 0 to 1", envThreshold, raw)
	}
	return float32(parsed)
}
