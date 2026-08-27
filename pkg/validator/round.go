package validator

import (
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// round tracks one height's in-flight consensus attempt: the tentatively
// applied batch (held open in txn, not yet visible to anyone else), the
// candidate block it produced, and the votes collected for it so far.
type round struct {
	height          uint64
	committee       []types.NFTID
	batch           []types.ShieldedTx
	txn             *state.Txn
	treeSnapshotLen int
	block           types.Block // Votes filled in only once quorum is reached
	candidate       types.Hash
	votes           []types.Vote
	deadline        time.Time
}

// rollback discards this round's tentative state changes: the held-open
// Badger transaction and any note-commitment tree appends.
func (r *round) rollback() {
	r.txn.Discard()
	// tree truncation happens in the caller, which owns the *state.MerkleTree
}
