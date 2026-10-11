package state

import "testing"

func TestPhaseOrder(t *testing.T) {
	if PhaseOrder(PhaseNotInstalled) != 0 {
		t.Errorf("PhaseOrder(NOT_INSTALLED) = %d", PhaseOrder(PhaseNotInstalled))
	}
	if PhaseOrder(PhaseCommitted) != 10 {
		t.Errorf("PhaseOrder(COMMITTED) = %d", PhaseOrder(PhaseCommitted))
	}
	if PhaseOrder("BOGUS") != -1 {
		t.Errorf("PhaseOrder(BOGUS) = %d, want -1", PhaseOrder("BOGUS"))
	}
}

func TestJournalForwardOnly(t *testing.T) {
	j, err := OpenJournal(t.TempDir(), "tx-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Transition(PhaseNotInstalled, PhasePreflightOK, "preflight"); err != nil {
		t.Fatalf("forward transition: %v", err)
	}
	if err := j.Transition(PhasePreflightOK, PhaseRuntimeStaged, "staged"); err != nil {
		t.Fatalf("forward transition: %v", err)
	}
	if err := j.Transition(PhaseRuntimeStaged, PhasePreflightOK, "regress"); err == nil {
		t.Fatal("backward transition was accepted")
	}
}

func TestJournalLastPhaseAndRecord(t *testing.T) {
	j, err := OpenJournal(t.TempDir(), "tx-2")
	if err != nil {
		t.Fatal(err)
	}
	if j.LastPhase() != PhaseNotInstalled {
		t.Errorf("empty journal LastPhase = %q", j.LastPhase())
	}
	if err := j.Record("informational"); err != nil {
		t.Fatal(err)
	}
	if j.LastPhase() != PhaseNotInstalled {
		t.Errorf("Record should not advance phase, got %q", j.LastPhase())
	}
	if err := j.Transition("", PhaseServiceCreated, "created"); err != nil {
		t.Fatal(err)
	}
	if j.LastPhase() != PhaseServiceCreated {
		t.Errorf("LastPhase = %q, want %q", j.LastPhase(), PhaseServiceCreated)
	}
}
