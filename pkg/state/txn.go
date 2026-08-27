package state

import (
	badger "github.com/dgraph-io/badger/v4"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Txn is a long-lived Badger transaction that batches many pkg/tx pipeline
// writes (across a whole proposed block) into one atomic commit-or-discard
// decision, instead of each write auto-committing the instant it happens
// (which is what *Store's methods do, and remain doing — unchanged — for
// callers that want that).
//
// Why this exists: real cross-node consensus (spec 5.7's BFT vote) means a
// validator must be able to tentatively apply a proposed batch, compute its
// resulting state root, and vote on it — all *before* knowing whether a
// quorum of the assigned committee will agree. If they don't (a network
// partition, a disagreeing minority, a timeout), those tentative writes
// must never become visible. Badger's own transactions already give exactly
// this all-or-nothing guarantee; Txn just keeps one open across that
// decision window instead of the one-shot db.Update the *Store methods use.
//
// A Txn is single-owner and not safe for concurrent use, matching how
// pkg/validator uses it: one round (one proposed block) in flight at a
// time per node.
type Txn struct {
	txn *badger.Txn
	key crypto.EncryptionKey
}

// Commit atomically applies every write made through this Txn.
func (t *Txn) Commit() error { return t.txn.Commit() }

// Discard abandons every write made through this Txn. Safe to call after a
// successful Commit (a no-op then, matching badger.Txn's own contract) —
// callers should still always `defer t.Discard()` right after BeginTxn.
func (t *Txn) Discard() { t.txn.Discard() }

func (t *Txn) MarkNullifierSpent(nullifier types.Hash) error {
	return markNullifierSpent(t.txn, nullifier)
}

func (t *Txn) IsNullifierSpent(nullifier types.Hash) (bool, error) {
	return isNullifierSpent(t.txn, nullifier)
}

func (t *Txn) PutNFT(nft types.ValidatorNFT) error {
	return setJSON(t.txn, prefixNFT, nft.ID.String(), nft)
}

func (t *Txn) GetNFT(id types.NFTID) (types.ValidatorNFT, bool, error) {
	var nft types.ValidatorNFT
	found, err := getJSON(t.txn, prefixNFT, id.String(), &nft)
	return nft, found, err
}

func (t *Txn) PutHold(hold types.BankHold) error {
	return setJSON(t.txn, prefixHold, hold.HoldID.String(), hold)
}

func (t *Txn) GetHold(id types.Hash) (types.BankHold, bool, error) {
	var hold types.BankHold
	found, err := getJSON(t.txn, prefixHold, id.String(), &hold)
	return hold, found, err
}

func (t *Txn) PutProposal(p ProposalRecord) error {
	return setJSON(t.txn, prefixProposal, p.ProposalID, p)
}

func (t *Txn) GetProposal(id string) (ProposalRecord, bool, error) {
	var p ProposalRecord
	found, err := getJSON(t.txn, prefixProposal, id, &p)
	return p, found, err
}

func (t *Txn) ListProposals() ([]ProposalRecord, error) {
	return listProposals(t.txn)
}

func (t *Txn) CountNFTs() (int, error) {
	return countPrefix(t.txn, prefixNFT)
}

func (t *Txn) PutContainerRoot(containerID string, root types.Hash) error {
	return setJSON(t.txn, prefixContainer, containerID, root)
}

func (t *Txn) GetContainerRoot(containerID string) (types.Hash, bool, error) {
	var root types.Hash
	found, err := getJSON(t.txn, prefixContainer, containerID, &root)
	return root, found, err
}
