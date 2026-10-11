package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Transaction phases from the specification state machine (section 12.1).
const (
	PhaseNotInstalled      = "NOT_INSTALLED"
	PhasePreflightOK       = "PREFLIGHT_OK"
	PhaseRuntimeStaged     = "RUNTIME_STAGED"
	PhaseRuntimeReady      = "RUNTIME_READY"
	PhaseDataInitializing  = "DATA_INITIALIZING"
	PhaseDataInitialized   = "DATA_INITIALIZED"
	PhaseServiceCreated    = "SERVICE_CREATED"
	PhaseACLVerified       = "ACL_VERIFIED"
	PhaseServiceStarting   = "SERVICE_STARTING"
	PhaseInstalledNotStart = "INSTALLED_NOT_STARTED"
	PhaseHealthy           = "HEALTHY"
	PhaseCommitted         = "COMMITTED"
)

// phaseOrder enforces that a transition only moves forward through the
// machine; a phase that is not listed is treated as unordered (recovery).
var phaseOrder = map[string]int{
	PhaseNotInstalled:      0,
	PhasePreflightOK:       1,
	PhaseRuntimeStaged:     2,
	PhaseRuntimeReady:      3,
	PhaseDataInitializing:  4,
	PhaseDataInitialized:   5,
	PhaseServiceCreated:    6,
	PhaseACLVerified:       7,
	PhaseServiceStarting:   8,
	PhaseInstalledNotStart: 9,
	PhaseHealthy:           9,
	PhaseCommitted:         10,
}

// PhaseOrder returns the numeric ordering of a phase for forward-only
// transition checks. Unknown phases return -1 and are treated as unordered.
func PhaseOrder(phase string) int {
	if n, ok := phaseOrder[phase]; ok {
		return n
	}
	return -1
}

// JournalEntry is one line of journal.jsonl.
type JournalEntry struct {
	Timestamp string `json:"timestamp"`
	TxID      string `json:"txid"`
	From      string `json:"from,omitempty"`
	To        string `json:"to"`
	Event     string `json:"event,omitempty"`
}

// Journal is an append-only transactional log.
type Journal struct {
	path string
	txid string
}

// OpenJournal opens (creating if needed) the journal for txid.
func OpenJournal(dir, txid string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Journal{path: filepath.Join(dir, "journal.jsonl"), txid: txid}, nil
}

// Transition appends a state-machine transition line. It validates that the
// requested move is forward-only, returning an error if an attempt is made
// to regress without an explicit event.
func (j *Journal) Transition(from, to, event string) error {
	if from != "" && to != "" && from != to {
		if a, ok := phaseOrder[from]; ok {
			if b, ok2 := phaseOrder[to]; ok2 && b < a {
				return fmt.Errorf("invalid state transition %s -> %s", from, to)
			}
		}
	}
	e := JournalEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TxID:      j.txid,
		From:      from,
		To:        to,
		Event:     event,
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Record appends an informational entry (no phase change).
func (j *Journal) Record(event string) error {
	return j.Transition("", "", event)
}

// LastPhase returns the most recent `to` phase recorded in the journal, or
// PhaseNotInstalled if the journal is empty or unreadable.
func (j *Journal) LastPhase() string {
	f, err := os.Open(j.path)
	if err != nil {
		return PhaseNotInstalled
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	last := PhaseNotInstalled
	for {
		var e JournalEntry
		if err := dec.Decode(&e); err != nil {
			break
		}
		if e.To != "" {
			last = e.To
		}
	}
	return last
}

// Path returns the journal file path.
func (j *Journal) Path() string { return j.path }
