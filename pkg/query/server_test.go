package query_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// testEnv wires up a real store + chain + mempool and starts a real query
// Server bound to a real loopback socket (port 0, OS-assigned) — every
// test in this file talks to it over an actual HTTP round trip, not an
// in-process handler call, so a real net/http client, real routing, and
// real middleware are all genuinely exercised.
type testEnv struct {
	store   *state.Store
	chn     *chain.Chain
	mempool *tx.Mempool
	base    string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("query-test-key-32-bytes-padding!"))
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

	return &testEnv{store: store, chn: chn, mempool: mempool, base: "http://" + srv.Addr()}
}

func (e *testEnv) get(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(e.base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for %s: %v", path, err)
	}
	return resp, body
}

// --- /v1/status ---

func TestStatusReflectsRealChainHead(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Height    uint64 `json:"height"`
		HeadHash  string `json:"head_hash"`
		GenesisMs int64  `json:"genesis_ms"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Height != env.chn.HeadHeight() {
		t.Fatalf("expected height %d, got %d", env.chn.HeadHeight(), got.Height)
	}
	if got.HeadHash != env.chn.HeadHash().String() {
		t.Fatalf("expected head hash %s, got %s", env.chn.HeadHash(), got.HeadHash)
	}
	if got.GenesisMs != 1735689600000 {
		t.Fatalf("unexpected genesis_ms: %d", got.GenesisMs)
	}
}

// --- /v1/blocks/{height} ---

func TestBlockReturnsGenesisAtHeightZero(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/blocks/0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got types.Block
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Height != 0 {
		t.Fatalf("expected genesis height 0, got %d", got.Height)
	}
}

func TestBlockNotFoundReturns404(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/blocks/999999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, body)
	}
}

func TestBlockRejectsNonNumericHeight(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/blocks/not-a-number")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- /v1/tx/{txid}: the real committed/pending/unknown tri-state ---

func TestTxStatusUnknownForNeverSeenTx(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/tx/"+strings.Repeat("00", 32))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != "unknown" {
		t.Fatalf("expected status unknown, got %q", got.Status)
	}
}

func TestTxStatusPendingForMempoolEntry(t *testing.T) {
	env := newTestEnv(t)
	txid := types.Hash{0x11}
	if err := env.mempool.Submit(types.ShieldedTx{TxID: txid}, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}

	resp, body := env.get(t, "/v1/tx/"+txid.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != "pending" {
		t.Fatalf("expected status pending, got %q", got.Status)
	}
}

// TestTxStatusCommittedAfterRealAppend drives an actual quorum-gated
// chain.Append (the same real path pkg/chain's own tests use) and proves
// the query API reports the real resulting height back — end to end
// through pkg/chain.Append's real indexing, not a mocked lookup.
func TestTxStatusCommittedAfterRealAppend(t *testing.T) {
	env := newTestEnv(t)

	type validatorKey struct {
		id types.NFTID
		pk crypto.DilithiumPublicKey
		sk crypto.DilithiumPrivateKey
	}
	genKey := func() validatorKey {
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		return validatorKey{id: types.NFTID(types.SumHash(pk)), pk: pk, sk: sk}
	}
	v1, v2, v3 := genKey(), genKey(), genKey()
	keys := map[types.NFTID]validatorKey{v1.id: v1, v2.id: v2, v3.id: v3}
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		k, ok := keys[id]
		return k.pk, ok
	}
	committee := []types.NFTID{v1.id, v2.id, v3.id}

	txid := types.Hash{0x22}
	batch := []types.ShieldedTx{{TxID: txid, Kind: types.TxVote}}
	b := env.chn.NextBlock(0, batch, types.Hash{9}, types.Hash{1}, types.Hash{}, v1.id, 100)
	candidate := types.HashBlock(b)
	// Real BFT-safe quorum for a 3-member committee is unanimous 3 of 3
	// (see consensus.BFTQuorumMet's own doc).
	for _, v := range []validatorKey{v1, v2, v3} {
		sig, err := crypto.DilithiumSign(v.sk, candidate[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		b.Votes = append(b.Votes, types.Vote{Validator: v.id, StateRoot: candidate, Sig: types.DilithiumSig(sig)})
	}
	if err := env.chn.Append(b, committee, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}

	resp, body := env.get(t, "/v1/tx/"+txid.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Status string  `json:"status"`
		Height *uint64 `json:"height"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != "committed" {
		t.Fatalf("expected status committed, got %q", got.Status)
	}
	if got.Height == nil || *got.Height != 1 {
		t.Fatalf("expected height 1, got %+v", got.Height)
	}
}

func TestTxRejectsMalformedTxID(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/tx/not-hex")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestTxRejectsValidHexWrongLength exercises the length-check branch of
// parseHash specifically — valid hex, but not 32 bytes of it — distinct
// from the not-valid-hex-at-all branch TestTxRejectsMalformedTxID covers.
func TestTxRejectsValidHexWrongLength(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/tx/aabb")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- /v1/nullifier/{hash} ---

func TestNullifierReflectsRealSpendState(t *testing.T) {
	env := newTestEnv(t)
	n := types.Hash{0x33}

	_, body := env.get(t, "/v1/nullifier/"+n.String())
	var got struct {
		Spent bool `json:"spent"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Spent {
		t.Fatalf("expected spent=false before marking spent")
	}

	if err := env.store.MarkNullifierSpent(n); err != nil {
		t.Fatalf("mark spent: %v", err)
	}
	resp, body := env.get(t, "/v1/nullifier/"+n.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Spent {
		t.Fatalf("expected spent=true after marking spent")
	}
}

// --- /v1/note/{commitment}: the critical privacy boundary ---

// TestNoteEndpointNeverLeaksPrivateFields is the safety-critical
// regression test this package's whole design turns on: GetNote decrypts
// a note's real Value/OwnerPk/Rho for the pipeline's internal use, and
// this proves none of that plaintext ever reaches an HTTP response, even
// though the note is real, stored, and genuinely retrievable server-side.
func TestNoteEndpointNeverLeaksPrivateFields(t *testing.T) {
	env := newTestEnv(t)

	note := types.Note{
		Commitment: types.Hash{0x44},
		Value:      123456789,
		OwnerPk:    []byte("super-secret-owner-public-key-material"),
		Rho:        []byte("super-secret-nullifier-seed"),
		Asset:      "SFG",
	}
	if err := env.store.PutNote(note); err != nil {
		t.Fatalf("put note: %v", err)
	}

	// Sanity: the store really does hold the decryptable plaintext value
	// (proves this test would actually catch a real leak, not a no-op).
	roundTrip, found, err := env.store.GetNote(note.Commitment)
	if err != nil || !found || roundTrip.Value != note.Value {
		t.Fatalf("expected the note to be really stored: found=%v err=%v value=%d", found, err, roundTrip.Value)
	}

	resp, body := env.get(t, "/v1/note/"+note.Commitment.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	raw := string(body)
	for _, secret := range []string{"123456789", "super-secret-owner-public-key-material", "super-secret-nullifier-seed"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("SAFETY VIOLATION: response leaked private note data (%q found in body): %s", secret, raw)
		}
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one field (exists) in the response, got %v", got)
	}
	exists, ok := got["exists"].(bool)
	if !ok || !exists {
		t.Fatalf("expected {\"exists\": true}, got %v", got)
	}
}

func TestNoteExistsFalseForUnknownCommitment(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/note/"+strings.Repeat("ab", 32))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Exists {
		t.Fatalf("expected exists=false for a commitment never stored")
	}
}

// --- /v1/nft/{id} ---

func TestNFTRoundTripsRealRecord(t *testing.T) {
	env := newTestEnv(t)
	nft := types.ValidatorNFT{ID: types.NFTID{0x55}, Owner: types.Address{0x01}, TP: 42, Traits: map[string]string{"dept": "Finance"}}
	if err := env.store.PutNFT(nft); err != nil {
		t.Fatalf("put nft: %v", err)
	}

	resp, body := env.get(t, "/v1/nft/"+nft.ID.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got types.ValidatorNFT
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TP != 42 || got.Traits["dept"] != "Finance" {
		t.Fatalf("unexpected nft data: %+v", got)
	}
}

func TestNFTNotFoundReturns404(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/nft/"+strings.Repeat("cd", 32))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// --- /v1/hold/{id} ---

func TestHoldRoundTripsRealRecord(t *testing.T) {
	env := newTestEnv(t)
	hold := types.BankHold{HoldID: types.Hash{0x66}, Owner: types.Address{0x02}, SFGIssued: 500}
	if err := env.store.PutHold(hold); err != nil {
		t.Fatalf("put hold: %v", err)
	}

	resp, body := env.get(t, "/v1/hold/"+hold.HoldID.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got types.BankHold
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SFGIssued != 500 {
		t.Fatalf("unexpected hold data: %+v", got)
	}
}

// --- /v1/proposal/{id} and /v1/proposals: the aggregate-only boundary ---

func TestProposalReturnsAggregateNotPerVoterData(t *testing.T) {
	env := newTestEnv(t)
	voter := types.Hash{0x77}
	rec := state.ProposalRecord{
		ProposalID:  "prop-1",
		Epoch:       3,
		Commitments: map[types.Hash]types.Hash{voter: {0x88}},
		Reveals:     map[types.Hash]bool{voter: true},
		Tallied:     true,
		Approve:     5,
		Reject:      2,
		Passed:      true,
	}
	if err := env.store.PutProposal(rec); err != nil {
		t.Fatalf("put proposal: %v", err)
	}

	resp, body := env.get(t, "/v1/proposal/prop-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	raw := string(body)
	if strings.Contains(raw, voter.String()) {
		t.Fatalf("SAFETY VIOLATION: response leaked a per-voter NFTID: %s", raw)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"Commitments", "Reveals", "commitments", "reveals"} {
		if _, present := got[forbidden]; present {
			t.Fatalf("SAFETY VIOLATION: response included a per-voter field %q: %v", forbidden, got)
		}
	}
	if got["approve"].(float64) != 5 || got["reject"].(float64) != 2 || got["passed"].(bool) != true {
		t.Fatalf("unexpected aggregate tally: %v", got)
	}
}

func TestProposalNotFoundReturns404(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/proposal/does-not-exist")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestProposalsListsAllReal(t *testing.T) {
	env := newTestEnv(t)
	for _, id := range []string{"prop-a", "prop-b"} {
		if err := env.store.PutProposal(state.ProposalRecord{ProposalID: id, Epoch: 1}); err != nil {
			t.Fatalf("put proposal %s: %v", id, err)
		}
	}
	resp, body := env.get(t, "/v1/proposals")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got []map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 proposals, got %d: %v", len(got), got)
	}
}

// --- cross-cutting: CORS, rate limiting ---

func TestCORSHeaderPresentOnGET(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/status")
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected CORS allow-origin *, got %q", got)
	}
}

func TestRateLimitEventuallyRejects(t *testing.T) {
	env := newTestEnv(t)
	// defaultBurst+defaultRateLimit are generous; hammering well past the
	// burst allowance in a tight loop must eventually surface a 429 —
	// proving the limiter is real middleware, not a decorative no-op.
	sawLimited := false
	for i := 0; i < 500; i++ {
		resp, _ := env.get(t, "/v1/status")
		if resp.StatusCode == http.StatusTooManyRequests {
			sawLimited = true
			break
		}
	}
	if !sawLimited {
		t.Fatalf("expected at least one 429 after 500 rapid requests from the same IP")
	}
}
