package state

import (
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
var (
	prefixCommitment = []byte("commit:")
	prefixNullifier  = []byte("null:")
	prefixNFT        = []byte("nft:")
	prefixHold       = []byte("hold:")
	prefixProposal   = []byte("prop:")
	prefixContainer  = []byte("container:")
)

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

func withPrefix(prefix []byte, id string) []byte {
	return append(append([]byte{}, prefix...), []byte(id)...)
}

func (s *Store) setJSON(prefix []byte, id string, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(withPrefix(prefix, id), b)
	})
}

func (s *Store) getJSON(prefix []byte, id string, v interface{}) (bool, error) {
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(withPrefix(prefix, id))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, v)
		})
	})
	if err != nil {
		return false, fmt.Errorf("state: get: %w", err)
	}
	return found, nil
}

// --- Notes (encrypted) ---

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

// MarkNullifierSpent records that a note's nullifier has been spent. It
// fails if the nullifier is already spent, enforcing the double-spend rule
// at the storage layer as a defense-in-depth check behind the circuit's own
// nullifier-derivation proof (spec 8.1).
func (s *Store) MarkNullifierSpent(nullifier types.Hash) error {
	return s.db.Update(func(txn *badger.Txn) error {
		key := withPrefix(prefixNullifier, nullifier.String())
		if _, err := txn.Get(key); err == nil {
			return fmt.Errorf("state: nullifier %s already spent (double-spend rejected)", nullifier)
		} else if err != badger.ErrKeyNotFound {
			return err
		}
		return txn.Set(key, []byte{1})
	})
}

func (s *Store) IsNullifierSpent(nullifier types.Hash) (bool, error) {
	spent := false
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(withPrefix(prefixNullifier, nullifier.String()))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		spent = true
		return nil
	})
	return spent, err
}

// --- NFTs ---

func (s *Store) PutNFT(nft types.ValidatorNFT) error {
	return s.setJSON(prefixNFT, nft.ID.String(), nft)
}

func (s *Store) GetNFT(id types.NFTID) (types.ValidatorNFT, bool, error) {
	var nft types.ValidatorNFT
	found, err := s.getJSON(prefixNFT, id.String(), &nft)
	return nft, found, err
}

// --- Bank holds ---

func (s *Store) PutHold(hold types.BankHold) error {
	return s.setJSON(prefixHold, hold.HoldID.String(), hold)
}

func (s *Store) GetHold(id types.Hash) (types.BankHold, bool, error) {
	var hold types.BankHold
	found, err := s.getJSON(prefixHold, id.String(), &hold)
	return hold, found, err
}

// --- Proposals (vote commitments) ---

type ProposalRecord struct {
	ProposalID  string
	Commitments []types.Hash
}

func (s *Store) PutProposal(p ProposalRecord) error {
	return s.setJSON(prefixProposal, p.ProposalID, p)
}

func (s *Store) GetProposal(id string) (ProposalRecord, bool, error) {
	var p ProposalRecord
	found, err := s.getJSON(prefixProposal, id, &p)
	return p, found, err
}

// --- Containers (subspace roots) ---

func (s *Store) PutContainerRoot(containerID string, root types.Hash) error {
	return s.setJSON(prefixContainer, containerID, root)
}

func (s *Store) GetContainerRoot(containerID string) (types.Hash, bool, error) {
	var root types.Hash
	found, err := s.getJSON(prefixContainer, containerID, &root)
	return root, found, err
}
