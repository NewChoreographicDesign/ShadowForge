package txclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/txclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func openStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("txclient-test-key-32-byte-pad!!!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustSignedVote(t *testing.T, proposalID string) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tx := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID(proposalID),
			Commitment: types.Hash{0x1},
		},
		Nullifier: types.Hash{0x2},
	}
	tx.TxID = types.ComputeTxID(tx.Proof, tx.Commitments, tx.Nullifier)
	sig, err := crypto.DilithiumSign(sk, tx.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tx.Sig = types.DilithiumSig(sig)
	tx.SignerPubKey = []byte(pk)
	return tx
}

func newConnectedPair(t *testing.T) (nodeA, nodeB *shadownet.Node, received chan shadownet.Envelope) {
	t.Helper()
	hostA, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host A: %v", err)
	}
	t.Cleanup(func() { _ = hostA.Close() })
	hostB, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host B: %v", err)
	}
	t.Cleanup(func() { _ = hostB.Close() })

	received = make(chan shadownet.Envelope, 8)
	nodeB = shadownet.NewNode(hostB, nil, func(p peer.ID, env shadownet.Envelope) {
		received <- env
	})
	nodeA = shadownet.NewNode(hostA, nil, nil)

	addrs := shadownet.FullAddr(hostB)
	if len(addrs) == 0 {
		t.Fatalf("host B has no listen addresses")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shadownet.Connect(ctx, hostA, addrs[0]); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return nodeA, nodeB, received
}

// --- Submit ---

func TestSubmitBroadcastsRealTxOffer(t *testing.T) {
	nodeA, _, received := newConnectedPair(t)
	c, err := txclient.New(txclient.Config{Net: nodeA})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	txn := mustSignedVote(t, "submit-proof")
	if err := c.Submit(context.Background(), txn); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case env := <-received:
		if env.Type != shadownet.MsgTxOffer {
			t.Fatalf("expected a TxOffer envelope, got %s", env.Type)
		}
		var payload shadownet.TxOfferPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Tx.TxID != txn.TxID {
			t.Fatalf("expected the received tx to match the submitted one")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for the peer to receive the TxOffer")
	}
}

func TestSubmitFailsWithNoConnectedPeers(t *testing.T) {
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	node := shadownet.NewNode(h, nil, nil)
	c, err := txclient.New(txclient.Config{Net: node})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := c.Submit(context.Background(), mustSignedVote(t, "lonely")); err == nil {
		t.Fatalf("expected Submit to fail with zero connected peers")
	}
}

func TestNewRejectsNilNet(t *testing.T) {
	if _, err := txclient.New(txclient.Config{}); err == nil {
		t.Fatalf("expected an error when Config.Net is nil")
	}
}

// --- QueryStatus against a real pkg/query.Server ---

func startQueryServer(t *testing.T, store *state.Store, chn *chain.Chain, mempool *tx.Mempool) string {
	t.Helper()
	srv := query.NewServer(store, chn, mempool, query.Config{ListenAddr: "127.0.0.1:0", Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)
	return "http://" + srv.Addr()
}

func TestQueryStatusUnknownForNeverSeenTx(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	base := startQueryServer(t, store, chn, tx.NewMempool())

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{base}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	st, err := c.QueryStatus(context.Background(), types.Hash{0xAA})
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if st.State != txclient.StatusUnknown {
		t.Fatalf("expected unknown, got %s", st.State)
	}
}

func TestQueryStatusPendingForMempoolEntry(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	mempool := tx.NewMempool()
	txn := mustSignedVote(t, "pending-proof")
	if err := mempool.Submit(txn, time.Now()); err != nil {
		t.Fatalf("submit to mempool: %v", err)
	}
	base := startQueryServer(t, store, chn, mempool)

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{base}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	st, err := c.QueryStatus(context.Background(), txn.TxID)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if st.State != txclient.StatusPending {
		t.Fatalf("expected pending, got %s", st.State)
	}
}

func TestQueryStatusCommittedForRealCommittedTx(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	txn := mustSignedVote(t, "committed-proof")
	height := commitOneRealBlock(t, chn, []types.ShieldedTx{txn})
	base := startQueryServer(t, store, chn, tx.NewMempool())

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{base}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	st, err := c.QueryStatus(context.Background(), txn.TxID)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if st.State != txclient.StatusCommitted {
		t.Fatalf("expected committed, got %s", st.State)
	}
	if st.Height == nil || *st.Height != height {
		t.Fatalf("expected height %d, got %+v", height, st.Height)
	}
}

func TestQueryStatusFailsWithNoEndpointsConfigured(t *testing.T) {
	c, err := txclient.New(txclient.Config{Net: loopbackNode(t)})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.QueryStatus(context.Background(), types.Hash{0x1}); err == nil {
		t.Fatalf("expected an error with no query endpoints configured")
	}
}

func TestQueryStatusToleratesOneUnreachableEndpoint(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	txn := mustSignedVote(t, "resilient-proof")
	height := commitOneRealBlock(t, chn, []types.ShieldedTx{txn})
	liveBase := startQueryServer(t, store, chn, tx.NewMempool())

	c, err := txclient.New(txclient.Config{
		Net:       loopbackNode(t),
		QueryURLs: []string{"http://127.0.0.1:1", liveBase}, // port 1: nothing listens there
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	st, err := c.QueryStatus(context.Background(), txn.TxID)
	if err != nil {
		t.Fatalf("expected the live endpoint to still answer despite the dead one: %v", err)
	}
	if st.State != txclient.StatusCommitted || st.Height == nil || *st.Height != height {
		t.Fatalf("unexpected status: %+v", st)
	}
}

// TestQueryStatusDetectsRealDisagreement wires two independent, real
// query.Server instances backed by two independent stores where the same
// TxID is (artificially, for this test) committed at two different
// heights — proving QueryStatus surfaces a real disagreement as an error
// rather than silently trusting whichever endpoint happened to answer
// first.
func TestQueryStatusDetectsRealDisagreement(t *testing.T) {
	txn := mustSignedVote(t, "disagreement-proof")

	storeA := openStore(t)
	chnA, err := chain.Open(storeA, 1)
	if err != nil {
		t.Fatalf("open chain A: %v", err)
	}
	heightA := commitOneRealBlock(t, chnA, []types.ShieldedTx{txn})
	baseA := startQueryServer(t, storeA, chnA, tx.NewMempool())

	storeB := openStore(t)
	chnB, err := chain.Open(storeB, 1)
	if err != nil {
		t.Fatalf("open chain B: %v", err)
	}
	// Commit a filler block first so this store's copy of the same TxID
	// lands at a genuinely different height than storeA's.
	commitOneRealBlock(t, chnB, nil)
	heightB := commitOneRealBlock(t, chnB, []types.ShieldedTx{txn})
	if heightA == heightB {
		t.Fatalf("test setup bug: expected the two stores to disagree on height")
	}
	baseB := startQueryServer(t, storeB, chnB, tx.NewMempool())

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{baseA, baseB}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.QueryStatus(context.Background(), txn.TxID); err == nil {
		t.Fatalf("expected QueryStatus to report an error when endpoints disagree on committed height")
	}
}

// TestQueryStatusAgreesAcrossIndependentEndpoints is the mirror of
// TestQueryStatusDetectsRealDisagreement: two independent, real
// query.Server instances that happen to agree (the realistic, common
// case — two honest nodes on the same live network) must not be treated
// as a disagreement just because they're separate server instances
// backed by separate stores.
func TestQueryStatusAgreesAcrossIndependentEndpoints(t *testing.T) {
	txn := mustSignedVote(t, "agreement-proof")

	storeA := openStore(t)
	chnA, err := chain.Open(storeA, 1)
	if err != nil {
		t.Fatalf("open chain A: %v", err)
	}
	heightA := commitOneRealBlock(t, chnA, []types.ShieldedTx{txn})
	baseA := startQueryServer(t, storeA, chnA, tx.NewMempool())

	storeB := openStore(t)
	chnB, err := chain.Open(storeB, 1)
	if err != nil {
		t.Fatalf("open chain B: %v", err)
	}
	heightB := commitOneRealBlock(t, chnB, []types.ShieldedTx{txn})
	baseB := startQueryServer(t, storeB, chnB, tx.NewMempool())
	if heightA != heightB {
		t.Fatalf("test setup bug: expected both stores to commit at the same height (%d vs %d)", heightA, heightB)
	}

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{baseA, baseB}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	st, err := c.QueryStatus(context.Background(), txn.TxID)
	if err != nil {
		t.Fatalf("expected two agreeing endpoints to produce no error: %v", err)
	}
	if st.State != txclient.StatusCommitted || st.Height == nil || *st.Height != heightA {
		t.Fatalf("unexpected aggregated status: %+v", st)
	}
}

// --- Confirm ---

func TestConfirmSucceedsOnceCommitted(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	mempool := tx.NewMempool()
	txn := mustSignedVote(t, "confirm-proof")
	if err := mempool.Submit(txn, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	base := startQueryServer(t, store, chn, mempool)

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{base}, PollInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		mempool.Remove([]types.Hash{txn.TxID})
		commitOneRealBlock(t, chn, []types.ShieldedTx{txn})
	}()

	st, err := c.Confirm(context.Background(), txn.TxID, 3*time.Second)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if st.State != txclient.StatusCommitted {
		t.Fatalf("expected committed, got %s", st.State)
	}
}

func TestConfirmTimesOutWithErrConfirmTimeout(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	base := startQueryServer(t, store, chn, tx.NewMempool())

	c, err := txclient.New(txclient.Config{Net: loopbackNode(t), QueryURLs: []string{base}, PollInterval: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = c.Confirm(context.Background(), types.Hash{0xEE}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected a timeout error")
	}
	if !errors.Is(err, txclient.ErrConfirmTimeout) {
		t.Fatalf("expected errors.Is(err, ErrConfirmTimeout), got %v", err)
	}
}

// TestSubmitAndConfirmPropagatesConfirmTimeoutError proves errors.Is
// still finds ErrConfirmTimeout after SubmitAndConfirm wraps Confirm's
// own wrapped error a second time — a real transaction that gets
// broadcast but never lands (e.g. dropped by every peer's pipeline) must
// still let a caller distinguish "timed out" from any other failure via
// errors.Is, not just by matching an error string.
func TestSubmitAndConfirmPropagatesConfirmTimeoutError(t *testing.T) {
	nodeA, _, received := newConnectedPair(t)
	// Drain so the peer's inbound channel never blocks Broadcast, but
	// deliberately never runs the tx through any pipeline or chain — it
	// must never appear as committed anywhere.
	go func() {
		for range received {
		}
	}()

	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	base := startQueryServer(t, store, chn, tx.NewMempool())

	c, err := txclient.New(txclient.Config{Net: nodeA, QueryURLs: []string{base}, PollInterval: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = c.SubmitAndConfirm(context.Background(), mustSignedVote(t, "never-lands"), 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected SubmitAndConfirm to fail for a transaction nothing ever commits")
	}
	if !errors.Is(err, txclient.ErrConfirmTimeout) {
		t.Fatalf("expected errors.Is(err, ErrConfirmTimeout) to hold through SubmitAndConfirm's own wrapping, got %v", err)
	}
}

// TestSubmitAndConfirmEndToEnd is the real, full loop this package exists
// for: a client submits a real transaction over a real libp2p connection
// to a peer that runs it through a real tx.Pipeline and commits a real
// block via chain.Append with a genuine 2-validator BFT quorum, and the
// client confirms it via a real pkg/query API — every piece real, wired
// together, nothing mocked.
func TestSubmitAndConfirmEndToEnd(t *testing.T) {
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	mempool := tx.NewMempool()
	pipeline := tx.NewPipeline(tx.Deps{Store: store, StateTree: state.NewMerkleTree()})

	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v1: %v", err)
	}
	v2pk, v2sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v2: %v", err)
	}
	v1id := types.NFTID(types.SumHash(v1pk))
	v2id := types.NFTID(types.SumHash(v2pk))
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		switch id {
		case v1id:
			return v1pk, true
		case v2id:
			return v2pk, true
		}
		return nil, false
	}

	nodeA, nodeB, received := newConnectedPair(t)
	_ = nodeB // its handler (below) is what matters, not direct use

	// A minimal, real "validator" reacting to a real TxOffer: run it
	// through the real pipeline, and if accepted, really commit it with a
	// genuine 2/2 signed quorum — the same Append real consensus uses.
	go func() {
		env := <-received
		if env.Type != shadownet.MsgTxOffer {
			return
		}
		var payload shadownet.TxOfferPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			t.Errorf("decode TxOffer: %v", err)
			return
		}
		results := pipeline.ProcessBatch([]tx.Entry{{Tx: payload.Tx}})
		if results[0].Error != nil {
			t.Errorf("pipeline rejected the submitted tx: %v", results[0].Error)
			return
		}
		b := chn.NextBlock(0, []types.ShieldedTx{payload.Tx}, types.Hash{1}, types.Hash{2}, types.Hash{}, v1id, time.Now().UnixMilli())
		candidate := types.HashBlock(b)
		sig1, err := crypto.DilithiumSign(v1sk, candidate[:])
		if err != nil {
			t.Errorf("sign v1: %v", err)
			return
		}
		sig2, err := crypto.DilithiumSign(v2sk, candidate[:])
		if err != nil {
			t.Errorf("sign v2: %v", err)
			return
		}
		b.Votes = []types.Vote{
			{Validator: v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig1)},
			{Validator: v2id, StateRoot: candidate, Sig: types.DilithiumSig(sig2)},
		}
		if err := chn.Append(b, []types.NFTID{v1id, v2id}, lookup); err != nil {
			t.Errorf("append: %v", err)
		}
	}()

	base := startQueryServer(t, store, chn, mempool)
	c, err := txclient.New(txclient.Config{Net: nodeA, QueryURLs: []string{base}, PollInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	txn := mustSignedVote(t, "e2e-proof")
	// Real voter eligibility (pkg/tx's requireEligibleVoter) is
	// unconditional: this Vote's signer needs a real, minted NFT before
	// the pipeline goroutine above will accept it.
	txnOwner := types.AddressFromPubkey(txn.SignerPubKey)
	if err := store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(txnOwner[:])), Owner: txnOwner}); err != nil {
		t.Fatalf("seed voter nft: %v", err)
	}
	st, err := c.SubmitAndConfirm(context.Background(), txn, 5*time.Second)
	if err != nil {
		t.Fatalf("submit and confirm: %v", err)
	}
	if st.State != txclient.StatusCommitted {
		t.Fatalf("expected committed, got %s", st.State)
	}
	if st.Height == nil || *st.Height != 1 {
		t.Fatalf("expected height 1, got %+v", st.Height)
	}
}

// loopbackNode is a throwaway libp2p node for tests that only need
// Client.net to be non-nil (QueryStatus/Confirm don't touch it at all).
func loopbackNode(t *testing.T) *shadownet.Node {
	t.Helper()
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return shadownet.NewNode(h, nil, nil)
}

// commitOneRealBlock appends one real, genuinely-quorum-signed block
// containing batch onto chn and returns its height — shared by tests that
// need a committed transaction without duplicating the vote-signing
// boilerplate.
func commitOneRealBlock(t *testing.T, chn *chain.Chain, batch []types.ShieldedTx) uint64 {
	t.Helper()
	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v1: %v", err)
	}
	v2pk, v2sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v2: %v", err)
	}
	v1id := types.NFTID(types.SumHash(v1pk))
	v2id := types.NFTID(types.SumHash(v2pk))
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		switch id {
		case v1id:
			return v1pk, true
		case v2id:
			return v2pk, true
		}
		return nil, false
	}

	b := chn.NextBlock(0, batch, types.Hash{1}, types.Hash{2}, types.Hash{}, v1id, time.Now().UnixMilli())
	candidate := types.HashBlock(b)
	sig1, err := crypto.DilithiumSign(v1sk, candidate[:])
	if err != nil {
		t.Fatalf("sign v1: %v", err)
	}
	sig2, err := crypto.DilithiumSign(v2sk, candidate[:])
	if err != nil {
		t.Fatalf("sign v2: %v", err)
	}
	b.Votes = []types.Vote{
		{Validator: v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig1)},
		{Validator: v2id, StateRoot: candidate, Sig: types.DilithiumSig(sig2)},
	}
	if err := chn.Append(b, []types.NFTID{v1id, v2id}, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}
	return b.Height
}
