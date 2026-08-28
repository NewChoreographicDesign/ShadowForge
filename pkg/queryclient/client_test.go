package queryclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// testEnv wires up a real store + chain + mempool and starts a real query
// Server bound to a real loopback socket — every test in this file talks
// to it over a genuine HTTP round trip via queryclient.Client, not a
// mocked transport.
type testEnv struct {
	store   *state.Store
	chn     *chain.Chain
	mempool *tx.Mempool
	client  *queryclient.Client
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("queryclient-test-key-32-bytes-p!"))
	store, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	chn, err := chain.Open(store, 1735689600000)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}

	mempool := tx.NewMempool()
	srv := query.NewServer(store, chn, mempool, query.Config{
		ListenAddr: "127.0.0.1:0",
		GenesisMs:  1735689600000,
		Logf:       t.Logf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)

	return &testEnv{store: store, chn: chn, mempool: mempool, client: queryclient.New("http://" + srv.Addr())}
}

func TestStatusReflectsRealChainHead(t *testing.T) {
	env := newTestEnv(t)
	st, err := env.client.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Height != env.chn.HeadHeight() {
		t.Fatalf("expected height %d, got %d", env.chn.HeadHeight(), st.Height)
	}
	if st.HeadHash != env.chn.HeadHash() {
		t.Fatalf("expected head hash %s, got %s", env.chn.HeadHash(), st.HeadHash)
	}
	if st.GenesisMs != 1735689600000 {
		t.Fatalf("unexpected genesis ms: %d", st.GenesisMs)
	}
}

func TestBlockReturnsGenesisAtHeightZero(t *testing.T) {
	env := newTestEnv(t)
	b, err := env.client.Block(context.Background(), 0)
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if b.Height != 0 {
		t.Fatalf("expected genesis height 0, got %d", b.Height)
	}
}

func TestBlockNotFoundReturnsErrNotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.client.Block(context.Background(), 999999)
	if !errors.Is(err, queryclient.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTxStatusUnknownForNeverSeenTx(t *testing.T) {
	env := newTestEnv(t)
	st, err := env.client.TxStatus(context.Background(), types.Hash{0x11})
	if err != nil {
		t.Fatalf("tx status: %v", err)
	}
	if st.Status != "unknown" {
		t.Fatalf("expected status unknown, got %q", st.Status)
	}
}

func TestTxStatusPendingForMempoolEntry(t *testing.T) {
	env := newTestEnv(t)
	txid := types.Hash{0x22}
	if err := env.mempool.Submit(types.ShieldedTx{TxID: txid}, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	st, err := env.client.TxStatus(context.Background(), txid)
	if err != nil {
		t.Fatalf("tx status: %v", err)
	}
	if st.Status != "pending" {
		t.Fatalf("expected pending, got %q", st.Status)
	}
}

func TestTxStatusCommittedAfterRealAppend(t *testing.T) {
	env := newTestEnv(t)

	genKey := func() (types.NFTID, crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey) {
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		return types.NFTID(types.SumHash(pk)), pk, sk
	}
	v1id, v1pk, v1sk := genKey()
	v2id, v2pk, v2sk := genKey()
	v3id, _, _ := genKey()
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		switch id {
		case v1id:
			return v1pk, true
		case v2id:
			return v2pk, true
		}
		return nil, false
	}
	committee := []types.NFTID{v1id, v2id, v3id}

	txid := types.Hash{0x33}
	batch := []types.ShieldedTx{{TxID: txid, Kind: types.TxVote}}
	b := env.chn.NextBlock(0, batch, types.Hash{9}, types.Hash{1}, types.Hash{}, v1id, 100)
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
	if err := env.chn.Append(b, committee, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}

	st, err := env.client.TxStatus(context.Background(), txid)
	if err != nil {
		t.Fatalf("tx status: %v", err)
	}
	if st.Status != "committed" {
		t.Fatalf("expected committed, got %q", st.Status)
	}
	if st.Height == nil || *st.Height != 1 {
		t.Fatalf("expected height 1, got %+v", st.Height)
	}
}

func TestNullifierReflectsRealSpendState(t *testing.T) {
	env := newTestEnv(t)
	n := types.Hash{0x44}
	spent, err := env.client.NullifierSpent(context.Background(), n)
	if err != nil {
		t.Fatalf("nullifier spent: %v", err)
	}
	if spent {
		t.Fatalf("expected spent=false before marking")
	}
	if err := env.store.MarkNullifierSpent(n); err != nil {
		t.Fatalf("mark spent: %v", err)
	}
	spent, err = env.client.NullifierSpent(context.Background(), n)
	if err != nil {
		t.Fatalf("nullifier spent: %v", err)
	}
	if !spent {
		t.Fatalf("expected spent=true after marking")
	}
}

func TestNoteExistsReflectsRealCommitment(t *testing.T) {
	env := newTestEnv(t)
	note := types.Note{Commitment: types.Hash{0x55}, Value: 42, Asset: "SFG"}
	if err := env.store.PutNote(note); err != nil {
		t.Fatalf("put note: %v", err)
	}
	exists, err := env.client.NoteExists(context.Background(), note.Commitment)
	if err != nil {
		t.Fatalf("note exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected exists=true for a real stored note")
	}
	exists, err = env.client.NoteExists(context.Background(), types.Hash{0x99})
	if err != nil {
		t.Fatalf("note exists: %v", err)
	}
	if exists {
		t.Fatalf("expected exists=false for an unknown commitment")
	}
}

func TestNFTRoundTripsRealRecord(t *testing.T) {
	env := newTestEnv(t)
	nft := types.ValidatorNFT{ID: types.NFTID{0x66}, Owner: types.Address{0x01}, TP: 42, Traits: map[string]string{"dept": "Finance"}}
	if err := env.store.PutNFT(nft); err != nil {
		t.Fatalf("put nft: %v", err)
	}
	got, err := env.client.NFT(context.Background(), nft.ID)
	if err != nil {
		t.Fatalf("nft: %v", err)
	}
	if got.TP != 42 || got.Traits["dept"] != "Finance" {
		t.Fatalf("unexpected nft data: %+v", got)
	}
}

func TestNFTNotFoundReturnsErrNotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.client.NFT(context.Background(), types.NFTID{0x77})
	if !errors.Is(err, queryclient.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestHoldRoundTripsRealRecord(t *testing.T) {
	env := newTestEnv(t)
	hold := types.BankHold{HoldID: types.Hash{0x88}, Owner: types.Address{0x02}, SFGIssued: 500}
	if err := env.store.PutHold(hold); err != nil {
		t.Fatalf("put hold: %v", err)
	}
	got, err := env.client.Hold(context.Background(), hold.HoldID)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if got.SFGIssued != 500 {
		t.Fatalf("unexpected hold data: %+v", got)
	}
}

func TestHoldNotFoundReturnsErrNotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.client.Hold(context.Background(), types.Hash{0x99})
	if !errors.Is(err, queryclient.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProposalReturnsRealAggregateTally(t *testing.T) {
	env := newTestEnv(t)
	voter := types.Hash{0xaa}
	rec := state.ProposalRecord{
		ProposalID:  "prop-1",
		Epoch:       3,
		Commitments: map[types.Hash]types.Hash{voter: {0xbb}},
		Reveals:     map[types.Hash]bool{voter: true},
		Tallied:     true,
		Approve:     5,
		Reject:      2,
		Passed:      true,
	}
	if err := env.store.PutProposal(rec); err != nil {
		t.Fatalf("put proposal: %v", err)
	}
	got, err := env.client.Proposal(context.Background(), "prop-1")
	if err != nil {
		t.Fatalf("proposal: %v", err)
	}
	if got.Approve != 5 || got.Reject != 2 || !got.Passed {
		t.Fatalf("unexpected aggregate tally: %+v", got)
	}
}

func TestProposalNotFoundReturnsErrNotFound(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.client.Proposal(context.Background(), "does-not-exist")
	if !errors.Is(err, queryclient.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProposalsListsAllReal(t *testing.T) {
	env := newTestEnv(t)
	for _, id := range []string{"prop-a", "prop-b"} {
		if err := env.store.PutProposal(state.ProposalRecord{ProposalID: id, Epoch: 1}); err != nil {
			t.Fatalf("put proposal %s: %v", id, err)
		}
	}
	got, err := env.client.Proposals(context.Background())
	if err != nil {
		t.Fatalf("proposals: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 proposals, got %d: %+v", len(got), got)
	}
}
