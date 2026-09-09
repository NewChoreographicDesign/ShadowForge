package query_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// queryRequestsMetric reads the real, current value of pkg/metrics'
// shadowforge_query_requests_total{path,status} series via Prometheus's
// own public Gather API — the same interface a real scrape uses —
// without pkg/query needing to expose any test-only hook of its own.
// Missing means never observed, not an error (a fresh counter series
// only exists once first incremented).
func queryRequestsMetric(t *testing.T, path, status string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "shadowforge_query_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			var gotPath, gotStatus string
			for _, l := range m.GetLabel() {
				switch l.GetName() {
				case "path":
					gotPath = l.GetValue()
				case "status":
					gotStatus = l.GetValue()
				}
			}
			if gotPath == path && gotStatus == status {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

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

// appendBlock commits one more real, quorum-gated block onto env's chain
// (mirroring TestTxStatusCommittedAfterRealAppend's own real Append
// path, generalized to a reusable helper) and returns its height — for
// tests that just need N real blocks to exist, not any particular
// content.
func appendBlock(t *testing.T, env *testEnv, batch []types.ShieldedTx) uint64 {
	t.Helper()
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
	v1, v2 := genKey(), genKey()
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		switch id {
		case v1.id:
			return v1.pk, true
		case v2.id:
			return v2.pk, true
		}
		return nil, false
	}
	b := env.chn.NextBlock(0, batch, types.Hash{9}, types.Hash{1}, types.Hash{}, v1.id, time.Now().UnixMilli())
	candidate := types.HashBlock(b)
	for _, v := range []validatorKey{v1, v2} {
		sig, err := crypto.DilithiumSign(v.sk, candidate[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		b.Votes = append(b.Votes, types.Vote{Validator: v.id, StateRoot: candidate, Sig: types.DilithiumSig(sig)})
	}
	if err := env.chn.Append(b, []types.NFTID{v1.id, v2.id}, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}
	return b.Height
}

// --- /v1/blocks: pagination ---

func TestBlocksListsRecentNewestFirst(t *testing.T) {
	env := newTestEnv(t)
	appendBlock(t, env, nil) // height 1
	appendBlock(t, env, nil) // height 2
	appendBlock(t, env, nil) // height 3

	resp, body := env.get(t, "/v1/blocks?limit=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Blocks  []types.Block `json:"blocks"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got.Blocks))
	}
	if got.Blocks[0].Height != 3 || got.Blocks[1].Height != 2 {
		t.Fatalf("expected newest-first [3,2], got [%d,%d]", got.Blocks[0].Height, got.Blocks[1].Height)
	}
	if !got.HasMore {
		t.Fatalf("expected has_more=true with genesis and height 1 still unpaged")
	}
}

func TestBlocksPaginatesWithBefore(t *testing.T) {
	env := newTestEnv(t)
	appendBlock(t, env, nil) // height 1
	appendBlock(t, env, nil) // height 2
	appendBlock(t, env, nil) // height 3

	resp, body := env.get(t, "/v1/blocks?limit=2&before=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Blocks  []types.Block `json:"blocks"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got.Blocks))
	}
	if got.Blocks[0].Height != 1 || got.Blocks[1].Height != 0 {
		t.Fatalf("expected [1,0] below cursor 2, got [%d,%d]", got.Blocks[0].Height, got.Blocks[1].Height)
	}
	if got.HasMore {
		t.Fatalf("expected has_more=false once genesis (height 0) is reached")
	}
}

func TestBlocksRejectsBadLimit(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := env.get(t, "/v1/blocks?limit=0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp, _ = env.get(t, "/v1/blocks?limit=not-a-number")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBlocksCapsExcessiveLimit(t *testing.T) {
	env := newTestEnv(t)
	for i := 0; i < 5; i++ {
		appendBlock(t, env, nil)
	}
	resp, body := env.get(t, "/v1/blocks?limit=100000")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Blocks []types.Block `json:"blocks"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Blocks) != 6 { // genesis (0) + 5 appended (1-5)
		t.Fatalf("expected 6 blocks (genesis + 5), got %d", len(got.Blocks))
	}
}

func TestBlocksOnFreshChainReturnsOnlyGenesis(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.get(t, "/v1/blocks")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Blocks  []types.Block `json:"blocks"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Height != 0 {
		t.Fatalf("expected only genesis on a fresh chain, got %+v", got.Blocks)
	}
	if got.HasMore {
		t.Fatalf("expected has_more=false on a fresh chain")
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
	for _, v := range []validatorKey{v1, v2} {
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

// TestTxStatusCommittedIncludesFullContent proves handleTx's enrichment:
// once a tx is committed, the response carries its own full, real
// content (the exact same object /v1/blocks/{height} already returns in
// full as part of that height's Batch — see handleBlock's own doc for
// why that's already safe), not just status/height.
func TestTxStatusCommittedIncludesFullContent(t *testing.T) {
	env := newTestEnv(t)
	txid := types.Hash{0x23}
	batch := []types.ShieldedTx{
		{TxID: types.Hash{0x01}, Kind: types.TxVote},
		{TxID: txid, Kind: types.TxNFTTransfer, NFTTransferPublicInputs: &types.NFTTransferPublicInputs{
			Target: types.NFTID{0x77}, NewOwner: types.Address{0x88},
		}},
	}
	height := appendBlock(t, env, batch)

	resp, body := env.get(t, "/v1/tx/"+txid.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Status string            `json:"status"`
		Height *uint64           `json:"height"`
		Tx     *types.ShieldedTx `json:"tx"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != "committed" || got.Height == nil || *got.Height != height {
		t.Fatalf("unexpected status/height: %+v", got)
	}
	if got.Tx == nil {
		t.Fatalf("expected tx content to be included once committed")
	}
	if got.Tx.TxID != txid || got.Tx.Kind != types.TxNFTTransfer {
		t.Fatalf("unexpected tx content: %+v", got.Tx)
	}
	if got.Tx.NFTTransferPublicInputs == nil || got.Tx.NFTTransferPublicInputs.Target != (types.NFTID{0x77}) {
		t.Fatalf("expected the real NFTTransferPublicInputs to round-trip, got %+v", got.Tx.NFTTransferPublicInputs)
	}
}

// TestTxStatusPendingOmitsContent proves a pending (mempool-only) tx
// never carries content in the response — Tx is populated purely from
// the real committed block, never from the mempool's own copy, so an
// unconfirmed transaction never appears to have already landed.
func TestTxStatusPendingOmitsContent(t *testing.T) {
	env := newTestEnv(t)
	txid := types.Hash{0x24}
	if err := env.mempool.Submit(types.ShieldedTx{TxID: txid, Kind: types.TxTransfer}, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	resp, body := env.get(t, "/v1/tx/"+txid.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := got["tx"]; present {
		t.Fatalf("SAFETY VIOLATION: a pending tx must never carry committed content: %v", got)
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

// --- cross-cutting: CORS, rate limiting, metrics ---

// TestRequestsObservedInMetricsWithLowCardinalityLabels proves real HTTP
// requests actually increment pkg/metrics' real counter (not a mock),
// and that distinct dynamic values (two different heights) collapse
// into the same route label rather than each minting its own series —
// the real defense against unbounded label cardinality on a public node
// that will be queried with many distinct heights/hashes/ids over its
// lifetime.
func TestRequestsObservedInMetricsWithLowCardinalityLabels(t *testing.T) {
	env := newTestEnv(t)
	appendBlock(t, env, nil) // real height 1, so both 0 and 1 exist
	before200 := queryRequestsMetric(t, "/v1/blocks/{height}", "200")
	before404 := queryRequestsMetric(t, "/v1/blocks/{height}", "404")

	env.get(t, "/v1/blocks/0")   // real, existing genesis height -> 200
	env.get(t, "/v1/blocks/1")   // a different height, same route label -> 200
	env.get(t, "/v1/blocks/999") // not found -> 404

	if got := queryRequestsMetric(t, "/v1/blocks/{height}", "200") - before200; got != 2 {
		t.Fatalf("expected 2 new 200s under the shared route label, got %v", got)
	}
	if got := queryRequestsMetric(t, "/v1/blocks/{height}", "404") - before404; got != 1 {
		t.Fatalf("expected 1 new 404 under the shared route label, got %v", got)
	}
	// The literal, unbounded heights must never have minted their own
	// label values.
	if got := queryRequestsMetric(t, "/v1/blocks/0", "200"); got != 0 {
		t.Fatalf("expected no series keyed by the literal path, got %v", got)
	}
}

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
