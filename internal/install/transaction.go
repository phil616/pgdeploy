package install

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

// NewTxID returns a short random transaction identifier.
func NewTxID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// Transaction binds the journal, logger, and paths for a single operation.
type Transaction struct {
	TxID    string
	Logger  *diagnostics.Logger
	Journal *state.Journal
	Paths   state.Paths
}

// OpenTransaction creates the journal for txid and returns a Transaction.
func OpenTransaction(paths state.Paths, txid string, logger *diagnostics.Logger) (*Transaction, error) {
	j, err := state.OpenJournal(paths.State, txid)
	if err != nil {
		return nil, err
	}
	return &Transaction{TxID: txid, Logger: logger, Journal: j, Paths: paths}, nil
}

// Transition records and logs a state-machine transition.
func (t *Transaction) Transition(from, to, event string) error {
	if t.Journal != nil {
		if err := t.Journal.Transition(from, to, event); err != nil {
			return err
		}
	}
	if t.Logger != nil {
		t.Logger.Info("transaction", event)
	}
	return nil
}

// Advance moves the current phase to target only when that is a forward move,
// so recovery re-runs never emit backward journal entries.
func (t *Transaction) Advance(cur *string, target, event string) error {
	if state.PhaseOrder(target) < state.PhaseOrder(*cur) {
		return nil
	}
	if err := t.Transition(*cur, target, event); err != nil {
		return err
	}
	*cur = target
	return nil
}
