package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Slice A (mutate-staged-v1.9): the fixed five-phase record.
//
// Every `dharness mutate --staged` run prints exactly one record naming each
// phase once, in fixed order: snapshot, classify, discover, related, stryker.
// A phase moves only from not started to running to completed or failed; the
// first terminal transition wins so deferred cleanup can never rewrite a
// verdict the run already recorded. Durations come from phaseNow, a
// package-level clock with a test restore closure, following the repository's
// seam rule (time cannot be controlled, so it is seamed; the filesystem is
// not).

type phaseLabel int

const (
	phaseSnapshot phaseLabel = iota
	phaseClassify
	phaseDiscover
	phaseRelated
	phaseStryker
)

var phaseNames = [5]string{"snapshot", "classify", "discover", "related", "stryker"}

type phaseOutcome int

const (
	outcomeNotStarted phaseOutcome = iota
	outcomeRunning
	outcomeCompleted
	outcomeFailed
)

type phaseState struct {
	outcome  phaseOutcome
	started  time.Time
	elapsed  time.Duration
	reason   string
	rendered bool
}

type phaseRecord struct {
	phases [5]phaseState
}

// phaseNow is the clock the record reads. Tests replace it through
// SetPhaseNowForTest; production always reads wall time.
var phaseNow = time.Now

// SetPhaseNowForTest replaces the record clock and returns a restore closure.
func SetPhaseNowForTest(now func() time.Time) func() {
	old := phaseNow
	phaseNow = now
	return func() { phaseNow = old }
}

func newPhaseRecord() *phaseRecord { return &phaseRecord{} }

func (r *phaseRecord) start(label phaseLabel) {
	st := &r.phases[label]
	if st.outcome != outcomeNotStarted {
		return
	}
	st.outcome = outcomeRunning
	st.started = phaseNow()
}

func (r *phaseRecord) complete(label phaseLabel) {
	st := &r.phases[label]
	if st.outcome == outcomeCompleted || st.outcome == outcomeFailed {
		return
	}
	if st.outcome == outcomeNotStarted {
		st.started = phaseNow()
	}
	st.elapsed = phaseNow().Sub(st.started)
	st.outcome = outcomeCompleted
}

func (r *phaseRecord) fail(label phaseLabel, err error) {
	st := &r.phases[label]
	if st.outcome == outcomeCompleted || st.outcome == outcomeFailed {
		return
	}
	st.outcome = outcomeFailed
	st.reason = oneLine(err.Error())
}

// failRunning marks whichever phase was underway when the command died with
// the command's own error, so the record never renders a phase stuck in a
// running state no output grammar defines.
func (r *phaseRecord) failRunning(err error) {
	for label := range r.phases {
		if r.phases[label].outcome == outcomeRunning {
			r.phases[label].outcome = outcomeFailed
			r.phases[label].reason = oneLine(err.Error())
			return
		}
	}
}

// oneLine collapses an error reason for the record. The returned error keeps
// its details; only the rendered reason is flattened.
func oneLine(reason string) string {
	reason = strings.ReplaceAll(reason, "\r\n", " ")
	reason = strings.ReplaceAll(reason, "\n", " ")
	reason = strings.ReplaceAll(reason, "\r", " ")
	return strings.Join(strings.Fields(reason), " ")
}

func (r *phaseRecord) render() string {
	var sb strings.Builder
	sb.WriteString("phases:")
	for label, name := range phaseNames {
		sb.WriteString(" · ")
		sb.WriteString(name)
		sb.WriteString(" ")
		st := r.phases[label]
		switch st.outcome {
		case outcomeCompleted:
			fmt.Fprintf(&sb, "%gs", st.elapsed.Seconds())
		case outcomeFailed:
			sb.WriteString("failed: ")
			sb.WriteString(st.reason)
		default:
			sb.WriteString("not started")
		}
	}
	out := sb.String()
	return strings.Replace(out, "phases: · ", "phases: ", 1)
}

// renderOnce prints the record exactly once; later calls are no-ops, so a
// deferred render cannot duplicate a record an early return already printed.
func (r *phaseRecord) renderOnce(w io.Writer) {
	for label := range r.phases {
		if r.phases[label].rendered {
			return
		}
	}
	for label := range r.phases {
		r.phases[label].rendered = true
	}
	fmt.Fprintln(w, r.render())
}
