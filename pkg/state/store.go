package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Key prefixes, one per row kind named in spec section 7:
//
//	commitment -> encrypted note blob
//	nullifier -> spent flag
//	nft id -> ValidatorNFT
//	hold id -> BankHold
//	proposal id -> vote commitments
//	container id -> subspace root
//
// block/head are this package's own addition, for the block persistence
// pkg/chain needs (spec 18.1 lists no explicit row for it, but section 7's
// state layer is where block history naturally lives alongside everything
// else Badger-backed).
var (
	prefixCommitment = []byte("commit:")
	prefixNullifier  = []byte("null:")
	prefixNFT        = []byte("nft:")
	prefixNFTByOwner = []byte("nft_owner:")
	prefixHold       = []byte("hold:")
	prefixProposal   = []byte("prop:")
	prefixContainer  = []byte("container:")
	prefixBlock      = []byte("block:")
	prefixTxIndex    = []byte("tx:")
	keyHead          = []byte("head")
)

// Accessor is the read/write surface pkg/tx's pipeline needs. Both *Store
// (auto-commits every call, its original behavior) and *Txn (batches many
// calls into one long-lived, explicitly committed-or-discarded Badger
// transaction) implement it — see txn.go's doc comment for why the
// pipeline needs both modes.
type Accessor interface {
	MarkNullifierSpent(nullifier types.Hash) error
	IsNullifierSpent(nullifier types.Hash) (bool, error)
	PutNFT(nft types.ValidatorNFT) error
	// DeleteNFT permanently removes nft — the real spec-10.3 "burn" slash
	// outcome (governance.SlashBurn). See deleteNFT's own doc for why
	// both index entries must go together.
	DeleteNFT(nft types.ValidatorNFT) error
	// TransferNFTOwner moves nft (already carrying its new Owner) away
	// from oldOwner's index entry — the real spec-10.1 ownership-change
	// counterpart of PutNFT. See transferNFTOwner's own doc.
	TransferNFTOwner(oldOwner types.Address, nft types.ValidatorNFT) error
	GetNFT(id types.NFTID) (types.ValidatorNFT, bool, error)
	// GetNFTByOwner looks up the (at most one, real "one per wallet")
	// ValidatorNFT record for owner — the real check TxNFTMint's Stage 4
	// and TxVote/TxVoteReveal's voter-eligibility check both need,
	// backed by a real secondary index (prefixNFTByOwner) rather than a
	// full scan over every minted NFT.
	GetNFTByOwner(owner types.Address) (types.ValidatorNFT, bool, error)
	PutHold(hold types.BankHold) error
	GetHold(id types.Hash) (types.BankHold, bool, error)
	PutProposal(p ProposalRecord) error
	GetProposal(id string) (ProposalRecord, bool, error)
	ListProposals() ([]ProposalRecord, error)
	CountNFTs() (int, error)
	PutContainerRoot(containerID string, root types.Hash) error
	GetContainerRoot(containerID string) (types.Hash, bool, error)
}

var _ Accessor = (*Store)(nil)
var _ Accessor = (*Txn)(nil)

// Store is the Badger-backed encrypted account/note KV store (spec 3.3,
// section 7). Note blobs and memos are sealed with the store's
// EncryptionKey before being written; everything else (nullifier spent
// flags, NFT records, holds, proposals, container roots) is plaintext at
// the storage layer because those rows are either already public
// (nullifiers, DA roots) or protected by the shielded-transaction pipeline
// upstream, not by storage-at-rest encryption.
type Store struct {
	db  *badger.DB
	key crypto.EncryptionKey
}

// Open opens (or creates) a Badger database at path. If inMemory is true,
// path is ignored and the DB is entirely in-memory (used for tests and the
// ShadeLang sandbox mock).
func Open(path string, inMemory bool, key crypto.EncryptionKey) (*Store, error) {
	opts := badger.DefaultOptions(path).WithLogger(nil)
	if inMemory {
		opts = opts.WithInMemory(true)
	}
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("state: open badger: %w", err)
	}
	return &Store{db: db, key: key}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// BeginTxn opens a long-lived, explicitly committed-or-discarded Badger
// transaction sharing this Store's encryption key. See txn.go.
func (s *Store) BeginTxn() *Txn {
	return &Txn{txn: s.db.NewTransaction(true), key: s.key}
}

func withPrefix(prefix []byte, id string) []byte {
	return append(append([]byte{}, prefix...), []byte(id)...)
}

func setJSON(txn *badger.Txn, prefix []byte, id string, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	return txn.Set(withPrefix(prefix, id), b)
}

func getJSON(txn *badger.Txn, prefix []byte, id string, v interface{}) (bool, error) {
	item, err := txn.Get(withPrefix(prefix, id))
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, v)
	}); err != nil {
		return false, err
	}
	return true, nil
}

// countPrefix returns how many keys carry the given prefix, without
// unmarshaling their values.
func countPrefix(txn *badger.Txn, prefix []byte) (int, error) {
	it := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
	defer it.Close()
	n := 0
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		n++
	}
	return n, nil
}

// --- Notes (encrypted). Only *Store exposes these: notes are written by a
// receiving wallet scanning commitments, not by the validating pipeline
// (which never learns note plaintext), so they don't need Txn's batched-
// round semantics. ---

// PutNote seals note under the store's encryption key (AAD = commitment
// bytes, so a ciphertext cannot be replayed under a different commitment
// key) and writes it keyed by commitment.
func (s *Store) PutNote(note types.Note) error {
	plain, err := json.Marshal(note)
	if err != nil {
		return fmt.Errorf("state: marshal note: %w", err)
	}
	blob, err := crypto.Encrypt(s.key, plain, note.Commitment[:])
	if err != nil {
		return fmt.Errorf("state: encrypt note: %w", err)
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(withPrefix(prefixCommitment, note.Commitment.String()), blob)
	})
}

// GetNote decrypts and returns the note at commitment.
func (s *Store) GetNote(commitment types.Hash) (types.Note, bool, error) {
	var note types.Note
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(withPrefix(prefixCommitment, commitment.String()))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return item.Value(func(blob []byte) error {
			plain, err := crypto.Decrypt(s.key, blob, commitment[:])
			if err != nil {
				return err
			}
			return json.Unmarshal(plain, &note)
		})
	})
	if err != nil {
		return types.Note{}, false, fmt.Errorf("state: get note: %w", err)
	}
	return note, found, nil
}

// --- Nullifiers ---

func markNullifierSpent(txn *badger.Txn, nullifier types.Hash) error {
	key := withPrefix(prefixNullifier, nullifier.String())
	if _, err := txn.Get(key); err == nil {
		return fmt.Errorf("state: nullifier %s already spent (double-spend rejected)", nullifier)
	} else if err != badger.ErrKeyNotFound {
		return err
	}
	return txn.Set(key, []byte{1})
}

func isNullifierSpent(txn *badger.Txn, nullifier types.Hash) (bool, error) {
	_, err := txn.Get(withPrefix(prefixNullifier, nullifier.String()))
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkNullifierSpent records that a note's nullifier has been spent. It
// fails if the nullifier is already spent, enforcing the double-spend rule
// at the storage layer as a defense-in-depth check behind the circuit's own
// nullifier-derivation proof (spec 8.1).
func (s *Store) MarkNullifierSpent(nullifier types.Hash) error {
	return s.db.Update(func(txn *badger.Txn) error { return markNullifierSpent(txn, nullifier) })
}

func (s *Store) IsNullifierSpent(nullifier types.Hash) (bool, error) {
	var spent bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		spent, err = isNullifierSpent(txn, nullifier)
		return err
	})
	return spent, err
}

// --- NFTs ---

// putNFT stores nft under both its own id and — the real secondary
// index GetNFTByOwner needs — its owner, so "does this wallet already
// hold a soulbound NFT" and "which real NFT does this wallet's vote
// belong to" are both real, indexed lookups rather than a full scan.
// Re-saving an existing NFT (e.g. TxNFTTrait's update path) naturally
// overwrites the same owner-index entry with the same id.
func putNFT(txn *badger.Txn, nft types.ValidatorNFT) error {
	if err := setJSON(txn, prefixNFT, nft.ID.String(), nft); err != nil {
		return err
	}
	return setJSON(txn, prefixNFTByOwner, nft.Owner.String(), nft.ID)
}

func getNFTByOwner(txn *badger.Txn, owner types.Address) (types.ValidatorNFT, bool, error) {
	var id types.NFTID
	found, err := getJSON(txn, prefixNFTByOwner, owner.String(), &id)
	if err != nil || !found {
		return types.ValidatorNFT{}, false, err
	}
	var nft types.ValidatorNFT
	found, err = getJSON(txn, prefixNFT, id.String(), &nft)
	return nft, found, err
}

// deleteNFT removes nft's own record and its owner-index entry — the
// real spec-10.3 "burn" slash outcome (governance.SlashBurn), as opposed
// to "freeze" (ApplySlash's Slashed=true, record kept, via putNFT/PutNFT
// above). Both index entries must go together, or a burned owner would
// be left permanently unable to mint a new NFT (GetNFTByOwner still
// finding a dangling id) while the id itself resolves to nothing.
func deleteNFT(txn *badger.Txn, nft types.ValidatorNFT) error {
	if err := txn.Delete(withPrefix(prefixNFT, nft.ID.String())); err != nil {
		return err
	}
	return txn.Delete(withPrefix(prefixNFTByOwner, nft.Owner.String()))
}

// transferNFTOwner moves nft (already carrying its new Owner) to a fresh
// owner-index entry and removes the old one — the real spec-10.1
// ownership-change bookkeeping putNFT alone cannot do, since it only
// ever writes the CURRENT Owner's index entry and has no way to know a
// previous one needs clearing. Without this, a wallet that transferred
// its NFT away would keep resolving via GetNFTByOwner to an NFT it no
// longer holds — a real Sybil-resistance bug (spec 10.1's "one per
// wallet" would silently stop being enforced for that wallet), not
// merely stale bookkeeping.
func transferNFTOwner(txn *badger.Txn, oldOwner types.Address, nft types.ValidatorNFT) error {
	if err := txn.Delete(withPrefix(prefixNFTByOwner, oldOwner.String())); err != nil {
		return err
	}
	return putNFT(txn, nft)
}

func (s *Store) PutNFT(nft types.ValidatorNFT) error {
	return s.db.Update(func(txn *badger.Txn) error { return putNFT(txn, nft) })
}

// DeleteNFT permanently removes nft — see deleteNFT's own doc.
func (s *Store) DeleteNFT(nft types.ValidatorNFT) error {
	return s.db.Update(func(txn *badger.Txn) error { return deleteNFT(txn, nft) })
}

// TransferNFTOwner moves nft (already carrying its new Owner) to oldOwner's
// successor — see transferNFTOwner's own doc.
func (s *Store) TransferNFTOwner(oldOwner types.Address, nft types.ValidatorNFT) error {
	return s.db.Update(func(txn *badger.Txn) error { return transferNFTOwner(txn, oldOwner, nft) })
}

func (s *Store) GetNFT(id types.NFTID) (types.ValidatorNFT, bool, error) {
	var nft types.ValidatorNFT
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		found, err = getJSON(txn, prefixNFT, id.String(), &nft)
		return err
	})
	return nft, found, err
}

func (s *Store) GetNFTByOwner(owner types.Address) (types.ValidatorNFT, bool, error) {
	var nft types.ValidatorNFT
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		nft, found, err = getNFTByOwner(txn, owner)
		return err
	})
	return nft, found, err
}

// --- Bank holds ---

func (s *Store) PutHold(hold types.BankHold) error {
	return s.db.Update(func(txn *badger.Txn) error { return setJSON(txn, prefixHold, hold.HoldID.String(), hold) })
}

func (s *Store) GetHold(id types.Hash) (types.BankHold, bool, error) {
	var hold types.BankHold
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		found, err = getJSON(txn, prefixHold, id.String(), &hold)
		return err
	})
	return hold, found, err
}

// --- Proposals (sealed-ballot vote commitments + reveals + tally) ---

// ProposalRecord tracks one proposal's ballots and, once tallied, its
// outcome. Commitments/Reveals are keyed by a real anonymous ZK
// eligibility proof's Nullifier (types.VoteEligibilityProof.Nullifier),
// not a voter identity (one NFT, one vote — spec 9.1): a second TxVote
// bearing the same nullifier is rejected as already-committed rather than
// silently overwriting the first, and a reveal can only ever open the
// exact ballot that same nullifier committed (see
// types.ComputeVoteCommitment). Keying by nullifier rather than NFTID is
// what makes this dedup real without ever naming which NFT cast a ballot.
//
// Epoch is stamped from whichever transaction first references this
// ProposalID (a TxVote's commit, in practice — this build has no
// separate "open a proposal" transaction kind); it's what a real
// epoch-boundary tally (pkg/validator) uses to decide a proposal is due.
type ProposalRecord struct {
	ProposalID  string
	Epoch       types.EpochNumber
	Commitments map[types.Hash]types.Hash
	Reveals     map[types.Hash]bool

	// ParamKey/NewValue optionally bind this proposal to a concrete
	// governance.Params field change, taken from whichever TxVote first
	// referenced ProposalID (types.VotePublicInputs' own doc explains why
	// only the first voter's claim sticks). Empty ParamKey means this
	// proposal carries no direct protocol effect for TallyDueProposals to
	// apply.
	ParamKey string
	NewValue string

	// MintAmount/MintOutCommit are a real spec-17.4 epoch mint's bound
	// claim, taken from whichever TxVote first referenced ProposalID —
	// see types.VotePublicInputs' own doc. MintAmount == 0 means this
	// proposal requests no mint. MintApplied mirrors Applied, but for the
	// mint execution step (real note-commitment insertion into the
	// canonical tree plus Vault fee collection): kept distinct from
	// Passed for the identical reason Applied is — TallyDueProposals only
	// ever visits an untallied proposal once, so this is the durable
	// record of whether that step already ran.
	MintAmount    uint64
	MintOutCommit types.Hash
	MintApplied   bool
	// MintStaked/StakePositionCommit are the staked-yield proposer path's
	// own counterpart of MintOutCommit — see types.VotePublicInputs.
	// MintStaked's own doc. MintApplied above is shared by both paths
	// (it marks whichever real execution step — note insertion for the
	// direct path, position insertion for the staked one — already ran);
	// MintStaked just says which one MintApplied refers to.
	MintStaked          bool
	StakePositionCommit types.Hash

	// SlashTargetNFT/SlashBurn are a real spec-10.3 slash proposal's
	// bound claim, taken from whichever TxVote first referenced
	// ProposalID — see types.VotePublicInputs.SlashTargetNFT's own doc.
	// SlashTargetNFT's zero value means this proposal requests no slash.
	// SlashApplied mirrors MintApplied/Applied: the durable record of
	// whether the real execution step (pkg/nft.ApplySlash, plus a real
	// DeleteNFT for the burn outcome) already ran.
	SlashTargetNFT types.NFTID
	SlashBurn      bool
	SlashApplied   bool

	// UnlockTransferTarget/UnlockTransferApplied are a real spec-10.1
	// transfer-unlock proposal's bound claim and execution status — see
	// types.VotePublicInputs.UnlockTransferTarget's own doc.
	// UnlockTransferTarget's zero value means this proposal requests no
	// unlock. UnlockTransferApplied mirrors SlashApplied: the durable
	// record of whether pkg/nft.UnlockTransfer already ran.
	UnlockTransferTarget  types.NFTID
	UnlockTransferApplied bool

	// Tallied/Approve/Reject/Passed are populated once, by the
	// epoch-boundary tally that runs when a committed block's Epoch
	// moves past this proposal's own Epoch. Turnout isn't recorded here:
	// this build has no enumeration of "eligible" (i.e. entitled to vote)
	// NFTs distinct from CountNFTs' total minted count, so a turnout
	// fraction would either double as an eligible-count or be fabricated;
	// Approve/Reject/Passed (simple majority, spec 17.4 names no turnout
	// floor) are real and complete without it.
	Tallied bool
	Approve int
	Reject  int
	Passed  bool
	// Applied records whether a passed ParamKey change was actually
	// applied to live governance state. Kept distinct from Passed since
	// application can fail independently (an unrecognized ParamKey or
	// unparseable NewValue from a malformed first vote) without that
	// failure retroactively meaning the vote itself didn't pass, and so
	// TallyDueProposals — which only ever visits an untallied proposal
	// once — has a durable record of whether the apply step already ran.
	Applied bool
}

func (s *Store) PutProposal(p ProposalRecord) error {
	return s.db.Update(func(txn *badger.Txn) error { return setJSON(txn, prefixProposal, p.ProposalID, p) })
}

func (s *Store) GetProposal(id string) (ProposalRecord, bool, error) {
	var p ProposalRecord
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		found, err = getJSON(txn, prefixProposal, id, &p)
		return err
	})
	return p, found, err
}

// listProposals unmarshals every record under prefixProposal.
func listProposals(txn *badger.Txn) ([]ProposalRecord, error) {
	it := txn.NewIterator(badger.IteratorOptions{Prefix: prefixProposal})
	defer it.Close()
	var out []ProposalRecord
	for it.Seek(prefixProposal); it.ValidForPrefix(prefixProposal); it.Next() {
		var p ProposalRecord
		if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &p) }); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) ListProposals() ([]ProposalRecord, error) {
	var out []ProposalRecord
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		out, err = listProposals(txn)
		return err
	})
	return out, err
}

// CountNFTs returns how many ValidatorNFTs have ever been minted.
func (s *Store) CountNFTs() (int, error) {
	var n int
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		n, err = countPrefix(txn, prefixNFT)
		return err
	})
	return n, err
}

// --- Containers (subspace roots) ---

func (s *Store) PutContainerRoot(containerID string, root types.Hash) error {
	return s.db.Update(func(txn *badger.Txn) error { return setJSON(txn, prefixContainer, containerID, root) })
}

func (s *Store) GetContainerRoot(containerID string) (types.Hash, bool, error) {
	var root types.Hash
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		found, err = getJSON(txn, prefixContainer, containerID, &root)
		return err
	})
	return root, found, err
}

// --- Blocks + chain head (pkg/chain) ---

func blockKey(height uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], height)
	return withPrefix(prefixBlock, string(b[:]))
}

func (s *Store) PutBlock(b types.Block) error {
	return s.db.Update(func(txn *badger.Txn) error {
		enc, err := json.Marshal(b)
		if err != nil {
			return fmt.Errorf("state: marshal block: %w", err)
		}
		return txn.Set(blockKey(b.Height), enc)
	})
}

func (s *Store) GetBlock(height uint64) (types.Block, bool, error) {
	var b types.Block
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(blockKey(height))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return item.Value(func(val []byte) error { return json.Unmarshal(val, &b) })
	})
	if err != nil {
		return types.Block{}, false, fmt.Errorf("state: get block: %w", err)
	}
	return b, found, nil
}

// SetHead records height as the current chain head height.
func (s *Store) SetHead(height uint64) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], height)
		return txn.Set(keyHead, b[:])
	})
}

// GetHead returns the current chain head height, or found=false if no
// block has ever been committed.
func (s *Store) GetHead() (uint64, bool, error) {
	var height uint64
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(keyHead)
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("state: corrupt head record")
			}
			height = binary.BigEndian.Uint64(val)
			return nil
		})
	})
	return height, found, err
}

// --- Transaction index (pkg/query) ---

// IndexTx records that txid was included in the block at height, so a
// later GetTxHeight can answer "did this transaction land, and where"
// without scanning every block. pkg/chain.Append calls this for every
// entry in a block's Batch once that block has genuinely reached BFT
// quorum and been persisted — the index only ever reflects transactions a
// real, quorum-verified block actually committed, never a merely-proposed
// or merely-pending one.
func (s *Store) IndexTx(txid types.Hash, height uint64) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], height)
		return txn.Set(withPrefix(prefixTxIndex, txid.String()), b[:])
	})
}

// GetTxHeight returns the height of the block that committed txid, or
// found=false if this node has no record of it (never seen, still only
// pending, or committed on a block this node hasn't indexed).
func (s *Store) GetTxHeight(txid types.Hash) (uint64, bool, error) {
	var height uint64
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(withPrefix(prefixTxIndex, txid.String()))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("state: corrupt tx index record for %s", txid)
			}
			height = binary.BigEndian.Uint64(val)
			return nil
		})
	})
	if err != nil {
		return 0, false, fmt.Errorf("state: get tx height: %w", err)
	}
	return height, found, nil
}
