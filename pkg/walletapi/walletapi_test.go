package walletapi

import (
	"context"
	"crypto/ecdh"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// testBackend is a real, single-validator "network" — a real
// state.Store, chain.Chain, and tx.Pipeline behind a real libp2p peer
// this package's own BuildAndSubmitTransfer connects and broadcasts to,
// plus a real pkg/query HTTP server every Balance/Status call reads
// from. Deliberately the milestone-1 slice of cmd/wallet's own
// newTestBackend (see cmd/wallet/main_test.go): only what real Kind
// Transfer processing needs, nothing mocked.
type testBackend struct {
	store    *state.Store
	chn      *chain.Chain
	pipeline *tx.Pipeline
	queryURL string
	addr     string

	v1id types.NFTID
	v1pk crypto.DilithiumPublicKey
	v1sk crypto.DilithiumPrivateKey
}

func newTestBackend(t *testing.T, storeKeyByte byte, zkSys *zk.System, zkTree *zk.Tree, zkRoots *zk.RootHistory) *testBackend {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("walletapi-backend-test-key-32by!"))
	key[31] = storeKeyByte
	store, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	genesisMs := time.Now().UnixMilli()
	chn, err := chain.Open(store, genesisMs)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}

	deps := tx.Deps{
		Store:     store,
		StateTree: state.NewMerkleTree(),
		ZK:        zkSys,
		ZKTree:    zkTree,
		ZKRoots:   zkRoots,
	}
	pipeline := tx.NewPipeline(deps)

	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen validator key: %v", err)
	}
	v1id := types.NFTID(types.SumHash(v1pk))

	b := &testBackend{store: store, chn: chn, pipeline: pipeline, v1id: v1id, v1pk: v1pk, v1sk: v1sk}

	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("backend host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	shadownet.NewNode(h, nil, b.handle)

	addrs := shadownet.FullAddr(h)
	if len(addrs) == 0 {
		t.Fatalf("backend host has no listen address")
	}
	b.addr = addrs[0]

	srv := query.NewServer(store, chn, tx.NewMempool(), query.Config{ListenAddr: "127.0.0.1:0", GenesisMs: genesisMs, Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)
	b.queryURL = "http://" + srv.Addr()

	return b
}

// handle drives a real TxOffer through the real pipeline and, if
// accepted, commits it via a real single-validator BFT quorum — the
// same lone-validator self-quorum this codebase's consensus package
// deliberately supports for cold-start.
func (b *testBackend) handle(_ peer.ID, env shadownet.Envelope) {
	if env.Type != shadownet.MsgTxOffer {
		return
	}
	var payload shadownet.TxOfferPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	b.commit(payload.Tx)
}

// commit drives txn through the real pipeline and, if accepted, appends
// a real, single-validator-signed block — used for real wire traffic
// (handle above).
func (b *testBackend) commit(txn types.ShieldedTx) bool {
	results := b.pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
	if results[0].Error != nil {
		return false
	}
	return b.appendDirect(txn) == nil
}

// appendDirect commits txn straight to the real chain, bypassing Stage 2
// well-formedness (signature, fee commitment) entirely — used only for
// this test's own one-time genesis-funding event, matching cmd/wallet's
// own identical end-to-end transfer test: a real chain's genesis
// coinbase is never itself a signed, fee-paying user transaction either.
func (b *testBackend) appendDirect(txn types.ShieldedTx) error {
	committee := []types.NFTID{b.v1id}
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		if id == b.v1id {
			return b.v1pk, true
		}
		return nil, false
	}
	blk := b.chn.NextBlock(0, []types.ShieldedTx{txn}, types.Hash{1}, types.Hash{2}, types.Hash{}, b.v1id, time.Now().UnixMilli())
	candidate := types.HashBlock(blk)
	sig, err := crypto.DilithiumSign(b.v1sk, candidate[:])
	if err != nil {
		return err
	}
	blk.Votes = []types.Vote{{Validator: b.v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig)}}
	return b.chn.Append(blk, committee, lookup)
}

// TestCreateIdentityBalanceAndTransferEndToEnd proves the milestone-1
// slice of pkg/walletapi end to end against a real running backend: two
// real identities created through CreateIdentity, one funded via a real
// committed genesis block (this build's own disclosed bootstrap gap —
// see pkg/shieldedwallet's doc — means every wallet still discovers it
// purely by Sync-replaying real chain data), a real Groth16-proven
// transfer built and submitted from A to B over a real libp2p connection
// to the backend, confirmed via the real query API, and both wallets'
// real post-transfer balances checked afterward.
func TestCreateIdentityBalanceAndTransferEndToEnd(t *testing.T) {
	zkSys, err := zk.Setup()
	if err != nil {
		t.Fatalf("zk setup: %v", err)
	}
	zkParamsPath := filepath.Join(t.TempDir(), "zk-params.bin")
	zkParamsFile, err := os.Create(zkParamsPath)
	if err != nil {
		t.Fatalf("create zk params file: %v", err)
	}
	if _, err := zkSys.WriteTo(zkParamsFile); err != nil {
		t.Fatalf("write zk params: %v", err)
	}
	if err := zkParamsFile.Close(); err != nil {
		t.Fatalf("close zk params file: %v", err)
	}
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	zkRoots := zk.NewRootHistory(initialRoot)
	backend := newTestBackend(t, 0x01, zkSys, zkTree, zkRoots)

	// Real Status, against a freshly opened chain.
	status, err := Status(backend.queryURL)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Height != 0 {
		t.Fatalf("expected height 0 on a fresh chain, got %d", status.Height)
	}

	// Two real identities, created exactly as a wallet app's UI would.
	senderPath := filepath.Join(t.TempDir(), "sender.json")
	senderID, err := CreateIdentity(senderPath, "sender-passphrase")
	if err != nil {
		t.Fatalf("CreateIdentity(sender): %v", err)
	}
	if _, err := CreateIdentity(senderPath, "sender-passphrase"); err == nil {
		t.Fatalf("expected CreateIdentity to refuse overwriting an existing keystore")
	}

	receiverPath := filepath.Join(t.TempDir(), "receiver.json")
	receiverID, err := CreateIdentity(receiverPath, "receiver-passphrase")
	if err != nil {
		t.Fatalf("CreateIdentity(receiver): %v", err)
	}

	// Fund the sender with a real genesis-funding block — the same
	// off-chain-bootstrap-free pattern cmd/wallet's own end-to-end
	// transfer test uses, since this build has no live mint mechanism
	// (see pkg/shieldedwallet's own doc).
	senderPubBytes, err := hex.DecodeString(senderID.ShieldedPublicKeyHex)
	if err != nil {
		t.Fatalf("decode sender shielded pubkey: %v", err)
	}
	senderShieldedPub, err := ecdh.X25519().NewPublicKey(senderPubBytes)
	if err != nil {
		t.Fatalf("parse sender shielded pubkey: %v", err)
	}

	values := []uint64{60, 40}
	secrets := make([]zk.NoteSecret, len(values))
	for i, v := range values {
		sk, err := zk.NewSpendKey()
		if err != nil {
			t.Fatal(err)
		}
		rho, err := zk.NewRho()
		if err != nil {
			t.Fatal(err)
		}
		secrets[i] = zk.NoteSecret{Value: v, OwnerSK: sk, Rho: rho}
	}
	outCommits := make([]types.Hash, len(secrets))
	receiverPubs := make([]*ecdh.PublicKey, len(secrets))
	for i, s := range secrets {
		outCommits[i] = types.Hash(zk.ToBytes32(s.Commitment()))
		if _, err := zkTree.Insert(s.Commitment()); err != nil {
			t.Fatalf("seed canonical tree: %v", err)
		}
		receiverPubs[i] = senderShieldedPub
	}
	root, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	zkRoots.Record(root)

	genesisMemo, err := shieldedwallet.EncryptMemos(receiverPubs, secrets)
	if err != nil {
		t.Fatalf("encrypt genesis memos: %v", err)
	}
	genesisTx := types.ShieldedTx{
		Kind:                 types.TxTransfer,
		Commitments:          outCommits,
		Nullifier:            types.SumHash([]byte("walletapi-test-genesis-funding-event")),
		TransferPublicInputs: &types.TransferPublicInputs{MerkleRoot: types.Hash(zk.ToBytes32(root)), OutCommits: outCommits},
		Memo:                 genesisMemo,
	}
	genesisTx.TxID = types.ComputeTxID(genesisTx.Proof, genesisTx.Commitments, genesisTx.Nullifier)
	if err := backend.appendDirect(genesisTx); err != nil {
		t.Fatalf("append genesis funding block: %v", err)
	}

	// Real Balance — must discover the genesis funding purely via Sync.
	bal, err := Balance(senderPath, "sender-passphrase", backend.queryURL)
	if err != nil {
		t.Fatalf("Balance(sender): %v", err)
	}
	if bal.Balance != 100 {
		t.Fatalf("expected sender balance 100 (60+40), got %d", bal.Balance)
	}
	if bal.Identity.IdentityHex != senderID.IdentityHex {
		t.Fatalf("expected Balance's reported identity to match CreateIdentity's, got %s vs %s", bal.Identity.IdentityHex, senderID.IdentityHex)
	}

	// A real, Groth16-proven transfer, submitted over a real libp2p
	// connection to the backend and confirmed via the real query API.
	result, err := BuildAndSubmitTransfer(senderPath, "sender-passphrase", backend.queryURL, "", backend.addr, zkParamsPath, receiverID.ShieldedPublicKeyHex, 70, 5, 10)
	if err != nil {
		t.Fatalf("BuildAndSubmitTransfer: %v", err)
	}
	if !result.Confirmed {
		t.Fatalf("expected the real transfer to confirm, got %+v", result)
	}
	if result.Height == 0 {
		t.Fatalf("expected a nonzero confirmed height, got %+v", result)
	}

	// Both real post-transfer balances.
	receiverBal, err := Balance(receiverPath, "receiver-passphrase", backend.queryURL)
	if err != nil {
		t.Fatalf("Balance(receiver): %v", err)
	}
	if receiverBal.Balance != 70 {
		t.Fatalf("expected receiver balance 70, got %d", receiverBal.Balance)
	}

	senderBalAfter, err := Balance(senderPath, "sender-passphrase", backend.queryURL)
	if err != nil {
		t.Fatalf("Balance(sender) after transfer: %v", err)
	}
	if senderBalAfter.Balance != 25 {
		t.Fatalf("expected sender's real change balance 25 (100-70-5), got %d", senderBalAfter.Balance)
	}
	if senderBalAfter.RootHex == "" {
		t.Fatalf("expected a nonempty synced canonical root")
	}

	status, err = Status(backend.queryURL)
	if err != nil {
		t.Fatalf("Status after transfer: %v", err)
	}
	if status.Height != result.Height {
		t.Fatalf("expected Status height %d to match the confirmed transfer height %d", status.Height, result.Height)
	}
}

// TestBuildAndSubmitTransferRequiresBootstrap proves the same real
// argument validation cmd/wallet's own submitTx enforces: no bootstrap
// peer means nothing to broadcast to.
func TestBuildAndSubmitTransferRequiresBootstrap(t *testing.T) {
	senderPath := filepath.Join(t.TempDir(), "sender.json")
	if _, err := CreateIdentity(senderPath, "sender-passphrase"); err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	_, err := BuildAndSubmitTransfer(senderPath, "sender-passphrase", "http://127.0.0.1:1", "", "", "zk-params.bin", hex.EncodeToString(make([]byte, 32)), 10, 1, 0)
	if err == nil {
		t.Fatalf("expected an error when bootstrapAddr is empty")
	}
}
