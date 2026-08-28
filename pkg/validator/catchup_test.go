package validator_test

import (
	"context"
	crand "crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/validator"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// mustBuildCatchUpMintTx builds a real, signed Kind NFTMint transaction
// for a fresh identity, attested by attestorPK/attestorSK — cheap and
// precondition-free (unlike Kind Transfer) to use purely as "real batch
// content" so a validator actually produces a fresh block per submission
// rather than an empty-batch no-op (maybePropose skips empty batches).
// Nullifier is a fresh random value, exactly like the real
// txbuilder.Builder.NFTMint (see its own doc on randomHash) — without a
// per-tx-unique Nullifier, every mint built here would share the same
// zero Proof/Commitments/Nullifier and therefore the identical
// types.ComputeTxID, so the mempool's own duplicate-TxID rejection
// (tx.ErrDuplicateTx) would silently swallow every mint after the first.
func mustBuildCatchUpMintTx(t *testing.T, attestorPK crypto.DilithiumPublicKey, attestorSK crypto.DilithiumPrivateKey, nonce uint64) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	owner := types.AddressFromPubkey(pk)
	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, owner, nonce, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	var nullifier types.Hash
	if _, err := crand.Read(nullifier[:]); err != nil {
		t.Fatalf("read random nullifier: %v", err)
	}
	voterSK := zk.DeriveVoterSK(sk)
	mintTx := types.ShieldedTx{
		Kind: types.TxNFTMint,
		NFTMintPublicInputs: &types.NFTMintPublicInputs{
			Owner:                 owner,
			Nonce:                 nonce,
			AttestationIssuedAtMs: att.IssuedAtMs,
			Attestor:              []byte(att.Attestor),
			AttestationSig:        types.DilithiumSig(att.Sig),
			VoterCommitment:       types.Hash(zk.ToBytes32(zk.VoterCommitment(voterSK))),
		},
		Nullifier: nullifier,
	}
	mintTx.TxID = types.ComputeTxID(mintTx.Proof, mintTx.Commitments, mintTx.Nullifier)
	sig, err := crypto.DilithiumSign(sk, mintTx.TxID[:])
	if err != nil {
		t.Fatalf("sign mint tx: %v", err)
	}
	mintTx.Sig = types.DilithiumSig(sig)
	mintTx.SignerPubKey = []byte(pk)
	return mintTx
}

// TestNodeCatchesUpAcrossMultipleBlocks is the real, wire-level proof
// this package's own multi-block catch-up mechanism works. Nodes A and C
// are two real, live validators (consensus.MinCommitteeSize is 2 — a
// single validator alone can never pass AssignCommittee's own floor, so
// real BFT quorum here genuinely needs both) that independently commit
// several real blocks together before node B ever connects. B is
// constructed and wired to both A and C only afterward, so it starts
// more than one block behind; it never calls Start, so it never
// heartbeats, is never selected into any committee, and never has to
// vote — it is purely a passive observer of real wire traffic. Once A/C
// commit one more real block and broadcast the announce, B — seeing an
// announced height far past its own NextHeight() — issues a real
// shadownet.MsgBlockRequest over the wire to whichever of them sent it,
// receives a real MsgBlockResponse, independently re-verifies and
// replays every block in it (the identical tryAdoptBlockLocked path a
// single announce gets), and its chain head converges with theirs —
// real bytes over real libp2p connections, not a direct method-call
// shortcut into another node's internals.
func TestNodeCatchesUpAcrossMultipleBlocks(t *testing.T) {
	genesisMs := time.Now().UnixMilli()
	cfg := validator.Config{
		BatchInterval:     100 * time.Millisecond,
		RoundTimeout:      6 * time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
		OnlineTimeout:     3 * time.Second,
		Genesis:           consensus.GenesisTime(genesisMs),
	}

	eligSys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("eligibility zk setup: %v", err)
	}
	attestorPK, attestorSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate attestor: %v", err)
	}
	trustedAttestors := []crypto.DilithiumPublicKey{attestorPK}

	newNode := func(idx int) (*validator.Node, host.Host) {
		h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
		if err != nil {
			t.Fatalf("node %d: new host: %v", idx, err)
		}
		t.Cleanup(func() { _ = h.Close() })
		store := openIntegrationStore(t, 100+idx)
		tree := state.NewMerkleTree()
		chn, err := chain.Open(store, genesisMs)
		if err != nil {
			t.Fatalf("node %d: open chain: %v", idx, err)
		}
		v := vault.New(vault.DefaultSplits())
		mempool := tx.NewMempool()
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("node %d: generate identity: %v", idx, err)
		}
		logf := func(format string, args ...interface{}) {
			t.Logf("node%d: "+format, append([]interface{}{idx}, args...)...)
		}
		n := validator.NewNode(cfg, h, nil, store, tree, chn, nil, v, nil, trustedAttestors, eligSys, nil, nil, nil, mempool, pk, sk, false, logf)
		return n, h
	}

	nodeA, hostA := newNode(0)
	nodeC, hostC := newNode(2)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	addrsC := shadownet.FullAddr(hostC)
	if len(addrsC) == 0 {
		t.Fatalf("node C has no listen addresses")
	}
	connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := shadownet.Connect(connectCtx, hostA, addrsC[0]); err != nil {
		connectCancel()
		t.Fatalf("connect node A -> node C: %v", err)
	}
	connectCancel()

	nodeA.Start(ctx)
	nodeC.Start(ctx)

	// A real, plain shadownet.Node (no validator.Node behind it) submits
	// every mint the same way a real wallet would: one TxOffer, sent to
	// exactly one node (A). Height % 2 rotates which of A/C is actually
	// deterministically assigned proposer (consensus.AssignCommittee), so
	// seeding only A's mempool directly (bypassing the wire) would starve
	// whichever height picks C — this instead exercises the same real
	// TxOffer gossip-forwarding path TestFourNodesConvergeOnSameChain
	// already proves (handleMessage's MsgTxOffer case rebroadcasting a
	// newly admitted offer to the sender's own peers), so the tx reaches
	// the real proposer for its height regardless of which one that is.
	walletHost, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("wallet host: %v", err)
	}
	t.Cleanup(func() { _ = walletHost.Close() })
	walletNode := shadownet.NewNode(walletHost, nil, nil)
	walletConnectCtx, walletConnectCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := shadownet.Connect(walletConnectCtx, walletHost, addrsC[0]); err != nil {
		walletConnectCancel()
		t.Fatalf("connect wallet -> node C: %v", err)
	}
	walletConnectCancel()
	addrsA := shadownet.FullAddr(hostA)
	if len(addrsA) == 0 {
		t.Fatalf("node A has no listen addresses")
	}
	walletConnectCtx2, walletConnectCancel2 := context.WithTimeout(ctx, 5*time.Second)
	if err := shadownet.Connect(walletConnectCtx2, walletHost, addrsA[0]); err != nil {
		walletConnectCancel2()
		t.Fatalf("connect wallet -> node A: %v", err)
	}
	walletConnectCancel2()
	submitMint := func(mintTx types.ShieldedTx) {
		t.Helper()
		env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: mintTx})
		if err != nil {
			t.Fatalf("build tx offer: %v", err)
		}
		if err := walletNode.Send(ctx, hostA.ID(), env); err != nil {
			t.Fatalf("send tx offer to node A: %v", err)
		}
	}

	// Let the heartbeat mesh fully converge before submitting anything,
	// so both nodes compute an identical online set (and therefore an
	// identical deterministic committee) for the first height — the
	// same precondition TestFourNodesConvergeOnSameChain relies on.
	time.Sleep(10 * cfg.HeartbeatInterval)

	// A and C commit several real blocks together (real 2-of-2 BFT
	// quorum, since MinCommitteeSize == 2 means neither can ever quorum
	// alone), one at a time, waiting for each to land on A before
	// submitting the next — real separate blocks, not one batched
	// proposal.
	const beforeBBlocks = 3
	for i := 0; i < beforeBBlocks; i++ {
		mintTx := mustBuildCatchUpMintTx(t, attestorPK, attestorSK, uint64(i+1))
		submitMint(mintTx)
		wantHeight := uint64(i + 1)
		deadline := time.Now().Add(8 * time.Second)
		for nodeA.Chain().HeadHeight() < wantHeight {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for A/C to reach height %d (A at %d, C at %d)", wantHeight, nodeA.Chain().HeadHeight(), nodeC.Chain().HeadHeight())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	// Let C catch up to A the ordinary (single-block-behind) way before
	// B ever enters the picture, so B's own eventual catch-up range is
	// unambiguous.
	deadline := time.Now().Add(8 * time.Second)
	for nodeC.Chain().HeadHeight() != nodeA.Chain().HeadHeight() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for C to converge with A before B joins (A=%d C=%d)", nodeA.Chain().HeadHeight(), nodeC.Chain().HeadHeight())
		}
		time.Sleep(20 * time.Millisecond)
	}
	heightBeforeB := nodeA.Chain().HeadHeight()
	if heightBeforeB < beforeBBlocks {
		t.Fatalf("expected A/C at height >= %d before B joins, got %d", beforeBBlocks, heightBeforeB)
	}

	// Node B is constructed and connected to both A and C now — its
	// stream handler is live (registered at construction, inside
	// validator.NewNode) even though B.Start is deliberately never
	// called, so B receives every real wire message sent to it
	// (including the next BlockAnnounce, from whichever of A/C proposes
	// it) without ever heartbeating, and therefore without ever being
	// selected into any committee A or C computes.
	nodeB, hostB := newNode(1)
	for _, h := range []host.Host{hostA, hostC} {
		addrs := shadownet.FullAddr(h)
		if len(addrs) == 0 {
			t.Fatalf("peer host has no listen addresses")
		}
		bConnectCtx, bConnectCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := shadownet.Connect(bConnectCtx, hostB, addrs[0]); err != nil {
			bConnectCancel()
			t.Fatalf("connect node B -> peer: %v", err)
		}
		bConnectCancel()
	}

	if nodeB.Chain().HeadHeight() != 0 {
		t.Fatalf("expected node B to start at height 0, got %d", nodeB.Chain().HeadHeight())
	}

	// One more real block from A/C — announced to B over the now-live
	// connections, at a height far past B's own NextHeight() (1), which
	// is exactly what must trigger real multi-block catch-up rather
	// than a silently-dropped announce.
	finalTx := mustBuildCatchUpMintTx(t, attestorPK, attestorSK, uint64(beforeBBlocks+1))
	submitMint(finalTx)

	wantFinalHeight := heightBeforeB + 1
	deadline = time.Now().Add(10 * time.Second)
	for nodeA.Chain().HeadHeight() < wantFinalHeight {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for A/C to commit the final block (A at %d, want %d)", nodeA.Chain().HeadHeight(), wantFinalHeight)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Real, wire-driven multi-block catch-up: no direct call into node
	// B's internals here, just waiting for B's own handling of real
	// BlockAnnounce/BlockRequest/BlockResponse traffic to converge it.
	deadline = time.Now().Add(10 * time.Second)
	for nodeB.Chain().HeadHeight() < wantFinalHeight {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for node B to catch up: at height %d, want %d", nodeB.Chain().HeadHeight(), wantFinalHeight)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if nodeB.Chain().HeadHash() != nodeA.Chain().HeadHash() {
		t.Fatalf("node B caught up to the right height but a different head hash: A=%s B=%s", nodeA.Chain().HeadHash(), nodeB.Chain().HeadHash())
	}
}
