package project

import "testing"

// The record is evidence, not progress: it holds what the measurement cost and
// what produced it, so a reader can tell whether it still describes the code.
func TestEvidenceSurvivesAWriteAndRead(t *testing.T) {
	p := Project{Root: t.TempDir()}

	if p.ReadEvidence().ScopedMutation != nil {
		t.Fatal("a fresh repository reported a measurement it never took")
	}

	if err := p.RecordScopedMutation("src/thing.ts", 25); err != nil {
		t.Fatalf("RecordScopedMutation() = %v", err)
	}

	measured := p.ReadEvidence().ScopedMutation
	if measured == nil {
		t.Fatal("the measurement did not survive the write")
	}
	if measured.RelatedTests != 25 || measured.MeasuredPath != "src/thing.ts" {
		t.Errorf("read back %+v", measured)
	}
	if measured.MeasuredAt.IsZero() {
		t.Error("the measurement carries no date, so nobody can tell how stale it is")
	}
}

// An unreadable record means the question is unanswered, which is the normal
// state of a new repository — not an error that should stop a command.
func TestUnreadableEvidenceReadsAsUnanswered(t *testing.T) {
	p := Project{Root: t.TempDir()}
	if err := p.RecordScopedMutation("src/a.ts", 1); err != nil {
		t.Fatal(err)
	}
	write(t, p.evidencePath(), "{ not json")

	if p.ReadEvidence().ScopedMutation != nil {
		t.Error("a corrupt record was read as a real measurement")
	}
}
