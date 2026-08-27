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
	prefixHold       = []byte("hold:")
	prefixProposal   = []byte("prop:")
	prefixContainer  = []byte("container:")
	prefixBlock      = []byte("block:")
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
	GetNFT(id types.NFTID) (types.ValidatorNFT, bool, error)
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

func (s *Store) PutNFT(nft types.ValidatorNFT) error {
	return s.db.Update(func(txn *badger.Txn) error { return setJSON(txn, prefixNFT, nft.ID.String(), nft) })
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
// outcome. Commitments/Reveals are keyed by voter NFTID (one NFT, one
// vote — spec 9.1): a second TxVote from the same voter is rejected as
// already-committed rather than silently overwriting the first, and a
// reveal can only ever open the exact ballot that voter committed (see
// types.ComputeVoteCommitment).
//
// Epoch is stamped from whichever transaction first references this
// ProposalID (a TxVote's commit, in practice — this build has no
// separate "open a proposal" transaction kind); it's what a real
// epoch-boundary tally (pkg/validator) uses to decide a proposal is due.
type ProposalRecord struct {
	ProposalID  string
	Epoch       types.EpochNumber
	Commitments map[types.NFTID]types.Hash
	Reveals     map[types.NFTID]bool

	// ParamKey/NewValue optionally bind this proposal to a concrete
	// governance.Params field change, taken from whichever TxVote first
	// referenced ProposalID (types.VotePublicInputs' own doc explains why
	// only the first voter's claim sticks). Empty ParamKey means this
	// proposal carries no direct protocol effect for TallyDueProposals to
	// apply.
	ParamKey string
	NewValue string

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
