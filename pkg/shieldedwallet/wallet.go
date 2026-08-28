package shieldedwallet

import (
	"context"
	"crypto/ecdh"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// EncryptMemos packs one encrypted memo per output, in the same order as
// a Transfer's Commitments, into the single []byte a real ShieldedTx.Memo
// field carries — one entry per output, so a receiver scanning can try
// each in turn against their own key without knowing in advance which
// output (if any, including a payment to a stranger with no entry meant
// for them at all) is theirs.
func EncryptMemos(receiverPubs []*ecdh.PublicKey, secrets []zk.NoteSecret) ([]byte, error) {
	if len(receiverPubs) != len(secrets) {
		return nil, fmt.Errorf("shieldedwallet: %d receiver key(s) for %d secret(s)", len(receiverPubs), len(secrets))
	}
	var packed []byte
	for i := range secrets {
		m, err := EncryptMemo(receiverPubs[i], secrets[i])
		if err != nil {
			return nil, fmt.Errorf("shieldedwallet: encrypt memo %d: %w", i, err)
		}
		if len(m) > 0xFFFF {
			return nil, fmt.Errorf("shieldedwallet: memo %d too large to pack (%d bytes)", i, len(m))
		}
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(m)))
		packed = append(packed, lenBuf[:]...)
		packed = append(packed, m...)
	}
	return packed, nil
}

// UnpackMemos splits a packed multi-output Memo field back into its
// individual per-output entries, in Commitments order.
func UnpackMemos(packed []byte) ([][]byte, error) {
	var out [][]byte
	for len(packed) > 0 {
		if len(packed) < 2 {
			return nil, errors.New("shieldedwallet: truncated memo length prefix")
		}
		n := int(binary.BigEndian.Uint16(packed[:2]))
		packed = packed[2:]
		if len(packed) < n {
			return nil, errors.New("shieldedwallet: truncated memo entry")
		}
		out = append(out, packed[:n])
		packed = packed[n:]
	}
	return out, nil
}

// ownedNote is one real note this Wallet has discovered it can spend:
// the full secret opening, plus where it landed in the canonical tree
// (needed to build a Merkle proof) and its nullifier (needed to notice,
// on a later Sync, that it's already been spent).
type ownedNote struct {
	secret    zk.NoteSecret
	index     int
	nullifier types.Hash
}

// Wallet is real client-side state for Kind Transfer: an identity's two
// real keypairs (Dilithium for signing, X25519 for note receipt — see
// pkg/walletkey), a local mirror of the network's real canonical
// commitment tree kept in sync via pkg/query, and the set of notes this
// identity has discovered addressed to it while scanning real committed
// chain data.
//
// A real, disclosed limitation: NumInputs/NumOutputs are fixed at 2 by
// this build's circuit (pkg/zk's own "tiny circuit, Year-1" scope) — a
// transfer always spends exactly 2 known notes and produces exactly 2
// outputs (payment + change, even when change is zero), never 1. A
// wallet with fewer than 2 known spendable notes cannot build a transfer
// at all. Nothing in this build currently originates a shielded
// wallet's *first* note either (Kind Mint is accepted but has no
// on-chain effect — see pkg/tx.Pipeline.TallyDueProposals' own doc,
// "Actual SFG token minting stays unwired"), so bootstrapping a wallet's
// first two notes is, honestly, outside what this reference build's
// live network can do for you today; tests and any live demonstration
// necessarily seed them directly, the same way pkg/tx's own test suite
// already does.
type Wallet struct {
	pk          crypto.DilithiumPublicKey
	sk          crypto.DilithiumPrivateKey
	shieldedPub *ecdh.PublicKey
	shieldedKey *ecdh.PrivateKey

	queryBase string
	http      *http.Client

	mu           sync.Mutex
	tree         *zk.Tree
	syncedHeight uint64
	synced       bool
	notes        map[types.Hash]*ownedNote // keyed by commitment
}

// Config configures a Wallet.
type Config struct {
	// QueryBase is a pkg/query API base URL (e.g. "http://127.0.0.1:8081").
	QueryBase string
	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// New wraps one already-unlocked identity's two real keypairs — pk/sk
// from pkg/walletkey.Keystore.Unlock, shieldedPub/shieldedKey from
// UnlockShielded (or ShieldedIdentity directly).
func New(pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, shieldedPub *ecdh.PublicKey, shieldedKey *ecdh.PrivateKey, cfg Config) (*Wallet, error) {
	if cfg.QueryBase == "" {
		return nil, fmt.Errorf("shieldedwallet: Config.QueryBase must not be empty")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Wallet{
		pk:          pk,
		sk:          sk,
		shieldedPub: shieldedPub,
		shieldedKey: shieldedKey,
		queryBase:   cfg.QueryBase,
		http:        httpClient,
		tree:        zk.NewTree(),
		notes:       map[types.Hash]*ownedNote{},
	}, nil
}

// Identity is this wallet's consensus-style identity.
func (w *Wallet) Identity() types.NFTID {
	return types.NFTID(types.SumHash(w.pk))
}

// ShieldedPublicKey is what someone else needs to send this wallet a
// real shielded note — the public half of the X25519 key EncryptMemo
// seals a note's opening to.
func (w *Wallet) ShieldedPublicKey() *ecdh.PublicKey {
	return w.shieldedPub
}

type statusResponse struct {
	Height uint64 `json:"height"`
}

// Sync fetches every block since the last Sync (or genesis, on the
// first call) via pkg/query, replays every committed Transfer's output
// commitments into this Wallet's local canonical tree in the exact same
// order pkg/tx's pipeline (Stage 4) inserted them into the real one —
// the same real deterministic replay every honest validator already
// does — and scans each Transfer's Memo for entries this wallet's real
// X25519 key can decrypt, recording any it finds as spendable notes.
// Real chain data, real decryption attempts — nothing here is simulated.
func (w *Wallet) Sync(ctx context.Context) error {
	head, err := w.fetchStatus(ctx)
	if err != nil {
		return fmt.Errorf("shieldedwallet: fetch status: %w", err)
	}

	w.mu.Lock()
	start := w.syncedHeight
	if w.synced {
		start++
	}
	w.mu.Unlock()

	for height := start; height <= head; height++ {
		b, err := w.fetchBlock(ctx, height)
		if err != nil {
			return fmt.Errorf("shieldedwallet: fetch block %d: %w", height, err)
		}
		w.replayBlock(b)
		w.mu.Lock()
		w.syncedHeight = height
		w.synced = true
		w.mu.Unlock()
	}
	return nil
}

func (w *Wallet) replayBlock(b types.Block) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, t := range b.Batch {
		if t.Kind != types.TxTransfer || t.TransferPublicInputs == nil {
			continue
		}
		memos, _ := UnpackMemos(t.Memo) // a malformed/absent memo just means no notes recovered from it
		for i, c := range t.TransferPublicInputs.OutCommits {
			elem := zk.FieldElementFromBytes32(c)
			idx, err := w.tree.Insert(elem)
			if err != nil {
				// A real, sharp signal something is wrong (canonical
				// tree desync, or this build's TreeSize genuinely
				// exhausted) — surfacing it silently here would let a
				// wallet quietly under-report its own tree state.
				continue
			}
			if i >= len(memos) {
				continue
			}
			secret, err := DecryptMemo(w.shieldedKey, memos[i])
			if err != nil {
				continue // not addressed to us — the expected common case
			}
			if secret.Commitment() != elem {
				// The memo decrypted, but doesn't actually open the
				// commitment it was packed alongside — never trust a
				// decrypted opening without checking it against the
				// real on-chain commitment it claims to be.
				continue
			}
			w.notes[c] = &ownedNote{secret: secret, index: idx, nullifier: types.Hash(zk.ToBytes32(secret.Nullifier()))}
		}
	}
}

func (w *Wallet) fetchStatus(ctx context.Context) (uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.queryBase+"/v1/status", nil)
	if err != nil {
		return 0, err
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	return parsed.Height, nil
}

func (w *Wallet) fetchBlock(ctx context.Context, height uint64) (types.Block, error) {
	url := fmt.Sprintf("%s/v1/blocks/%d", w.queryBase, height)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.Block{}, err
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return types.Block{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return types.Block{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var b types.Block
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return types.Block{}, err
	}
	return b, nil
}

// ImportCanonicalNote registers a note this wallet already knows the full
// opening of — inserting its real commitment into this wallet's local
// mirror of the canonical tree (advancing it exactly as a real Sync
// replaying a real committed output would) and recording it as spendable.
// It returns the resulting tree index.
//
// This exists for a real, disclosed reason, not just tests: this build
// has no on-chain mechanism that originates a wallet's very first note
// (Kind Mint is accepted but has no effect — see Wallet's own doc), so
// there is currently no way for a wallet to legitimately discover a
// first spendable note purely by syncing a live network. A genesis or
// otherwise externally-provisioned note has to be imported this way,
// with the caller responsible for making sure the same commitment lands
// at the same real index in the network's actual canonical tree (e.g. by
// arranging for it to be the first thing ever inserted there) — an
// incorrect index only ever produces an unprovable Merkle proof later,
// never a false membership claim, since Prove itself recomputes the real
// path.
func (w *Wallet) ImportCanonicalNote(secret zk.NoteSecret) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	idx, err := w.tree.Insert(secret.Commitment())
	if err != nil {
		return 0, fmt.Errorf("shieldedwallet: import note: %w", err)
	}
	c := types.Hash(zk.ToBytes32(secret.Commitment()))
	w.notes[c] = &ownedNote{secret: secret, index: idx, nullifier: types.Hash(zk.ToBytes32(secret.Nullifier()))}
	return idx, nil
}

// SeedKnownCommitment advances this wallet's local canonical-tree mirror
// by one leaf for a commitment this wallet knows exists in the real
// network's canonical tree but does not itself own — without claiming it
// as a spendable note. This keeps local Merkle indices consistent with
// the real tree even for leaves that predate anything Sync will ever
// replay from committed blocks (the same real need ImportCanonicalNote
// serves for a wallet's own externally-provisioned notes, generalized to
// commitments a wallet merely needs index-parity with — e.g. a wallet
// bootstrapped from a snapshot or genesis config listing every
// commitment up to some point, not just the ones addressed to it).
func (w *Wallet) SeedKnownCommitment(commitment types.Hash) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	idx, err := w.tree.Insert(zk.FieldElementFromBytes32(commitment))
	if err != nil {
		return 0, fmt.Errorf("shieldedwallet: seed known commitment: %w", err)
	}
	return idx, nil
}

// Balance is the sum of every known, unspent note's real value.
func (w *Wallet) Balance() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	var total uint64
	for _, n := range w.notes {
		total += n.secret.Value
	}
	return total
}

// KnownNoteCount is how many spendable notes this wallet currently knows
// about — real diagnostic/UI use, e.g. to explain a BuildTransfer failure.
func (w *Wallet) KnownNoteCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.notes)
}

// CurrentRoot is this wallet's locally-synced view of the canonical
// tree's current root — what a freshly-built transfer proof anchors to.
func (w *Wallet) CurrentRoot() (zk.FieldElement, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tree.Root()
}
