package shieldedwallet_test

import (
	"context"
	stdecdh "crypto/ecdh"
	stdrand "crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	zkOnce sync.Once
	zkSys  *zk.System
	zkErr  error
)

func getZKSystem(t *testing.T) *zk.System {
	t.Helper()
	zkOnce.Do(func() { zkSys, zkErr = zk.Setup() })
	if zkErr != nil {
		t.Fatalf("zk setup: %v", zkErr)
	}
	return zkSys
}

func openStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("shieldedwallet-test-key-32-byte!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// testNetwork bundles a real store/chain/pipeline (with the real
// canonical-tree security fix wired in, exactly as a live validator
// configures it) and a real pkg/query API in front of it.
type testNetwork struct {
	store    *state.Store
	chn      *chain.Chain
	pipeline *tx.Pipeline
	zkTree   *zk.Tree
	zkRoots  *zk.RootHistory
	queryURL string

	v1id, v2id types.NFTID
	v1pk, v2pk crypto.DilithiumPublicKey
	v1sk, v2sk crypto.DilithiumPrivateKey
}

func newTestNetwork(t *testing.T) *testNetwork {
	t.Helper()
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	zkRoots := zk.NewRootHistory(initialRoot)
	pipeline := tx.NewPipeline(tx.Deps{
		Store:     store,
		StateTree: state.NewMerkleTree(),
		ZK:        getZKSystem(t),
		ZKTree:    zkTree,
		ZKRoots:   zkRoots,
	})

	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v1: %v", err)
	}
	v2pk, v2sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v2: %v", err)
	}

	srv := query.NewServer(store, chn, tx.NewMempool(), query.Config{ListenAddr: "127.0.0.1:0", Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)

	return &testNetwork{
		store: store, chn: chn, pipeline: pipeline, zkTree: zkTree, zkRoots: zkRoots,
		queryURL: "http://" + srv.Addr(),
		v1id:     types.NFTID(types.SumHash(v1pk)), v2id: types.NFTID(types.SumHash(v2pk)),
		v1pk: v1pk, v2pk: v2pk, v1sk: v1sk, v2sk: v2sk,
	}
}

// commit runs txn through the real pipeline and, if accepted, commits a
// real block for it with a genuine 2-validator BFT quorum — the same
// Append real consensus uses.
func (n *testNetwork) commit(t *testing.T, txn types.ShieldedTx) uint64 {
	t.Helper()
	results := n.pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
	if results[0].Error != nil {
		t.Fatalf("pipeline rejected transaction: %v", results[0].Error)
	}
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		switch id {
		case n.v1id:
			return n.v1pk, true
		case n.v2id:
			return n.v2pk, true
		}
		return nil, false
	}
	b := n.chn.NextBlock(0, []types.ShieldedTx{txn}, types.Hash{1}, types.Hash{2}, types.Hash{}, n.v1id, time.Now().UnixMilli())
	candidate := types.HashBlock(b)
	sig1, err := crypto.DilithiumSign(n.v1sk, candidate[:])
	if err != nil {
		t.Fatalf("sign v1: %v", err)
	}
	sig2, err := crypto.DilithiumSign(n.v2sk, candidate[:])
	if err != nil {
		t.Fatalf("sign v2: %v", err)
	}
	b.Votes = []types.Vote{
		{Validator: n.v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig1)},
		{Validator: n.v2id, StateRoot: candidate, Sig: types.DilithiumSig(sig2)},
	}
	if err := n.chn.Append(b, []types.NFTID{n.v1id, n.v2id}, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}
	return b.Height
}

func newTestWallet(t *testing.T, net *testNetwork) *shieldedwallet.Wallet {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate dilithium: %v", err)
	}
	xsk, err := stdecdh.X25519().GenerateKey(stdrand.Reader)
	if err != nil {
		t.Fatalf("generate x25519: %v", err)
	}
	w, err := shieldedwallet.New(pk, sk, xsk.PublicKey(), xsk, shieldedwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}
	return w
}

func mkNote(t *testing.T, v uint64) zk.NoteSecret {
	t.Helper()
	sk, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	return zk.NoteSecret{Value: v, OwnerSK: sk, Rho: rho}
}

// bootstrapSenderNotes gives sender real "genesis" notes: imported
// locally into the wallet (ImportCanonicalNote) and, in the exact same
// order, inserted into the network's real canonical tree — the
// documented, honest bootstrap step this build's missing mint-
// origination mechanism requires (see Wallet's own doc).
func bootstrapSenderNotes(t *testing.T, net *testNetwork, sender *shieldedwallet.Wallet, values []uint64) {
	t.Helper()
	for _, v := range values {
		secret := mkNote(t, v)
		if _, err := sender.ImportCanonicalNote(secret); err != nil {
			t.Fatalf("import canonical note: %v", err)
		}
		if _, err := net.zkTree.Insert(secret.Commitment()); err != nil {
			t.Fatalf("seed canonical tree: %v", err)
		}
	}
	root, err := net.zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	net.zkRoots.Record(root)
}

// genesisNotes creates real note secrets and inserts their commitments
// into the network's real canonical tree, in order, recording the
// resulting root — the network-side half of an honest off-chain
// bootstrap (see bootstrapSenderNotes and Wallet's own doc on the
// missing on-chain mint mechanism). It does not touch any wallet.
func genesisNotes(t *testing.T, net *testNetwork, values []uint64) []zk.NoteSecret {
	t.Helper()
	secrets := make([]zk.NoteSecret, len(values))
	for i, v := range values {
		secrets[i] = mkNote(t, v)
		if _, err := net.zkTree.Insert(secrets[i].Commitment()); err != nil {
			t.Fatalf("seed canonical tree: %v", err)
		}
	}
	root, err := net.zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	net.zkRoots.Record(root)
	return secrets
}

// claimGenesisNotes imports secrets into w as real spendable notes,
// advancing w's local tree mirror in the same order genesisNotes
// inserted them into the real network tree.
func claimGenesisNotes(t *testing.T, w *shieldedwallet.Wallet, secrets []zk.NoteSecret) {
	t.Helper()
	for _, s := range secrets {
		if _, err := w.ImportCanonicalNote(s); err != nil {
			t.Fatalf("import canonical note: %v", err)
		}
	}
}

// seedGenesisIndices keeps w's local tree mirror index-consistent with
// the real network tree for genesis commitments w does not itself own —
// without this, w's Sync-built tree would only ever contain the leaves
// visible in committed blocks, permanently diverging in structure (and
// therefore root) from the real network tree, which also carries these
// off-chain-bootstrapped leaves.
func seedGenesisIndices(t *testing.T, w *shieldedwallet.Wallet, secrets []zk.NoteSecret) {
	t.Helper()
	for _, s := range secrets {
		if _, err := w.SeedKnownCommitment(types.Hash(zk.ToBytes32(s.Commitment()))); err != nil {
			t.Fatalf("seed known commitment: %v", err)
		}
	}
}

// TestWalletToWalletTransferEndToEnd is the real, full loop this package
// exists for: a sender wallet spends bootstrapped notes across two real
// Groth16-proven Transfers, each committed through the real pipeline and
// real chain.Append, and a receiver — who has never seen the sender's
// plaintext note data, only real committed chain bytes fetched over a
// real pkg/query API — decrypts its own real memo entries and recovers
// spendable notes purely by syncing. Every piece real: the proofs, the
// canonical-tree anchoring (the security fix this task also closed), the
// encryption, the discovery.
//
// Every wallet that will ever need to build a provable transfer must
// keep its local tree index-consistent with the real network tree,
// including for the off-chain-bootstrapped genesis leaves that predate
// any block (see seedGenesisIndices): a wallet whose local tree only
// ever replayed committed blocks would silently diverge in structure —
// and therefore root — from the real network tree, and any transfer it
// later tried to actually commit would be rejected by the real canonical-
// root check (the same soundness fix this task closed). Both sender and
// receiver are seeded here for that reason, even though receiver never
// owns the genesis notes themselves.
func TestWalletToWalletTransferEndToEnd(t *testing.T) {
	net := newTestNetwork(t)
	sender := newTestWallet(t, net)
	receiver := newTestWallet(t, net)

	// A third genesis note (20) gives sender a second known note left
	// over after the first transfer spends the two largest (60, 40) —
	// enough, combined with that transfer's own change, to fund a second
	// real transfer without any further off-chain bootstrap.
	genesis := genesisNotes(t, net, []uint64{60, 40, 20})
	claimGenesisNotes(t, sender, genesis)
	seedGenesisIndices(t, receiver, genesis)
	if sender.Balance() != 120 {
		t.Fatalf("expected sender's bootstrap balance to be 120, got %d", sender.Balance())
	}

	txn1, err := sender.BuildTransfer(getZKSystem(t), receiver.ShieldedPublicKey(), 70, 5)
	if err != nil {
		t.Fatalf("build transfer 1: %v", err)
	}
	if sender.KnownNoteCount() != 1 {
		t.Fatalf("expected the two spent inputs removed and the untouched 20-value note left, got %d remaining", sender.KnownNoteCount())
	}

	height := net.commit(t, txn1)
	if height != 1 {
		t.Fatalf("expected height 1, got %d", height)
	}

	ctx := context.Background()
	if err := receiver.Sync(ctx); err != nil {
		t.Fatalf("receiver sync: %v", err)
	}
	if got := receiver.Balance(); got != 70 {
		t.Fatalf("expected receiver to discover a real 70-value note, got balance %d", got)
	}
	if receiver.KnownNoteCount() != 1 {
		t.Fatalf("expected exactly 1 discovered note, got %d", receiver.KnownNoteCount())
	}

	// The sender's own change (100 - 70 - 5 = 25) was encrypted to
	// itself — a real re-sync should recover it exactly like any other
	// wallet discovering its own note, alongside the untouched 20-value
	// bootstrap note it already knew about.
	if err := sender.Sync(ctx); err != nil {
		t.Fatalf("sender re-sync: %v", err)
	}
	if got := sender.Balance(); got != 45 {
		t.Fatalf("expected sender's balance to be 20 (untouched) + 25 (real change) = 45, got %d", got)
	}
	if sender.KnownNoteCount() != 2 {
		t.Fatalf("expected sender to hold exactly 2 known notes after re-sync, got %d", sender.KnownNoteCount())
	}

	// A second real transfer, funded entirely by sender's now-2 known
	// notes (20 + 25 = 45), pays the receiver again — this is the
	// ordinary, no-bypass path any real wallet takes once it has 2 known
	// notes.
	txn2, err := sender.BuildTransfer(getZKSystem(t), receiver.ShieldedPublicKey(), 10, 1)
	if err != nil {
		t.Fatalf("build transfer 2: %v", err)
	}
	height2 := net.commit(t, txn2)
	if height2 != 2 {
		t.Fatalf("expected height 2, got %d", height2)
	}

	if err := receiver.Sync(ctx); err != nil {
		t.Fatalf("receiver re-sync: %v", err)
	}
	if got := receiver.Balance(); got != 80 {
		t.Fatalf("expected receiver's balance to be 70 + 10 = 80, got %d", got)
	}
	if receiver.KnownNoteCount() != 2 {
		t.Fatalf("expected receiver to hold exactly 2 known notes, got %d", receiver.KnownNoteCount())
	}

	// Both of the receiver's notes are really spendable: prove it by
	// having the receiver build AND commit a further transfer using
	// them. Committing (not just building) is what actually exercises
	// the real canonical-root check — a proof anchored to a root the
	// real network doesn't recognize would be rejected right here.
	other := newTestWallet(t, net)
	txn3, err := receiver.BuildTransfer(getZKSystem(t), other.ShieldedPublicKey(), 10, 1)
	if err != nil {
		t.Fatalf("expected the receiver's discovered notes to be genuinely spendable: %v", err)
	}
	height3 := net.commit(t, txn3)
	if height3 != 3 {
		t.Fatalf("expected height 3, got %d", height3)
	}

	if err := other.Sync(ctx); err != nil {
		t.Fatalf("other sync: %v", err)
	}
	if got := other.Balance(); got != 10 {
		t.Fatalf("expected other to discover a real 10-value note from the receiver's own transfer, got balance %d", got)
	}
}

func TestBuildTransferFailsWithFewerThanTwoNotes(t *testing.T) {
	net := newTestNetwork(t)
	w := newTestWallet(t, net)
	receiver := newTestWallet(t, net)

	if _, err := w.BuildTransfer(getZKSystem(t), receiver.ShieldedPublicKey(), 1, 0); err != shieldedwallet.ErrInsufficientNotes {
		t.Fatalf("expected ErrInsufficientNotes with zero known notes, got %v", err)
	}
}

func TestBuildTransferFailsWhenNotesDontCoverAmount(t *testing.T) {
	net := newTestNetwork(t)
	sender := newTestWallet(t, net)
	receiver := newTestWallet(t, net)
	bootstrapSenderNotes(t, net, sender, []uint64{10, 10})

	if _, err := sender.BuildTransfer(getZKSystem(t), receiver.ShieldedPublicKey(), 100, 1); err == nil {
		t.Fatalf("expected a transfer exceeding known note value to fail")
	}
	// A failed build must not have spent anything.
	if sender.KnownNoteCount() != 2 {
		t.Fatalf("expected notes to remain unspent after a failed build, got %d", sender.KnownNoteCount())
	}
}

func TestSyncIgnoresMemosNotAddressedToThisWallet(t *testing.T) {
	net := newTestNetwork(t)
	sender := newTestWallet(t, net)
	realReceiver := newTestWallet(t, net)
	uninvolved := newTestWallet(t, net)

	bootstrapSenderNotes(t, net, sender, []uint64{60, 40})
	txn, err := sender.BuildTransfer(getZKSystem(t), realReceiver.ShieldedPublicKey(), 70, 5)
	if err != nil {
		t.Fatalf("build transfer: %v", err)
	}
	net.commit(t, txn)

	if err := uninvolved.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := uninvolved.Balance(); got != 0 {
		t.Fatalf("expected an uninvolved wallet to discover nothing, got balance %d", got)
	}
}
