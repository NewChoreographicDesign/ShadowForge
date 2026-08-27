package validator_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/validator"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
)

// TestFourNodesConvergeOnSameChain is the real multi-node proof this
// package exists for: four independent validator.Node instances, each
// with its own libp2p host bound to its own loopback TCP port, its own
// Badger-backed store, and its own real Dilithium identity keypair, wired
// together over genuine Noise-encrypted libp2p connections — no shared
// memory between them except the network sockets. One transaction is
// submitted, and the test proves all four nodes independently reach BFT
// quorum and land on byte-identical chain state (height and head hash)
// purely through the wire protocol: real heartbeat-built committee
// assignment, real signed StageVote exchange, real chain.Append
// re-verification on every node — nothing here calls another node's
// internals directly, unlike this package's white-box unit tests.
func TestFourNodesConvergeOnSameChain(t *testing.T) {
	const n = 4
	genesisMs := time.Now().UnixMilli()

	cfg := validator.Config{
		BatchInterval:     100 * time.Millisecond,
		RoundTimeout:      6 * time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
		OnlineTimeout:     3 * time.Second,
		Genesis:           consensus.GenesisTime(genesisMs),
	}

	nodes := make([]*validator.Node, n)
	hosts := make([]host.Host, n)
	for i := 0; i < n; i++ {
		h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
		if err != nil {
			t.Fatalf("node %d: new host: %v", i, err)
		}
		t.Cleanup(func() { _ = h.Close() })
		hosts[i] = h

		store := openIntegrationStore(t, i)
		tree := state.NewMerkleTree()
		chn, err := chain.Open(store, genesisMs)
		if err != nil {
			t.Fatalf("node %d: open chain: %v", i, err)
		}
		v := vault.New(vault.DefaultSplits())
		mempool := tx.NewMempool()
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("node %d: generate identity key: %v", i, err)
		}

		idx := i
		logf := func(format string, args ...interface{}) {
			t.Logf("node%d: "+format, append([]interface{}{idx}, args...)...)
		}
		nodes[i] = validator.NewNode(cfg, h, nil, store, tree, chn, nil, v, mempool, pk, sk, logf)
	}

	// Full mesh: every node dials every other node, so Broadcast (which
	// only sends to n.Host.Network().Peers(), i.e. peers it has an open
	// connection with) actually reaches everyone.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			addrs := shadownet.FullAddr(hosts[j])
			if len(addrs) == 0 {
				t.Fatalf("node %d has no listen addresses", j)
			}
			connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := shadownet.Connect(connectCtx, hosts[i], addrs[0]); err != nil {
				connectCancel()
				t.Fatalf("connect node %d -> node %d: %v", i, j, err)
			}
			connectCancel()
		}
	}

	for _, node := range nodes {
		node.Start(ctx)
	}

	// Let the heartbeat mesh fully converge (every node has heard every
	// other node at least a few times) before introducing a transaction,
	// so every node computes an identical online set and therefore an
	// identical deterministic committee for height 1 — avoiding a race
	// where different nodes see different online sets and propose
	// conflicting candidates for the same height.
	time.Sleep(10 * cfg.HeartbeatInterval)

	// Submitted like a real wallet would: one TxOffer, sent to exactly one
	// node (node 0), over a connection none of the other three nodes are
	// party to. Whichever node ends up deterministically assigned
	// proposer for height 1 needs this transaction in ITS OWN mempool to
	// propose it — so this also exercises pkg/validator's TxOffer gossip
	// forwarding (handleMessage's MsgTxOffer case rebroadcasting a newly
	// admitted offer to its own peers), not just a shortcut that seeds
	// every node's mempool directly.
	walletHost, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("wallet host: %v", err)
	}
	t.Cleanup(func() { _ = walletHost.Close() })
	walletNode := shadownet.NewNode(walletHost, nil, nil)
	addrs := shadownet.FullAddr(hosts[0])
	if len(addrs) == 0 {
		t.Fatalf("node 0 has no listen addresses")
	}
	connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := shadownet.Connect(connectCtx, walletHost, addrs[0]); err != nil {
		connectCancel()
		t.Fatalf("wallet connect to node 0: %v", err)
	}
	connectCancel()

	voteTx := mustSignIntegrationVote(t, "integration-proposal")
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: voteTx})
	if err != nil {
		t.Fatalf("build tx offer: %v", err)
	}
	if err := walletNode.Send(ctx, hosts[0].ID(), env); err != nil {
		t.Fatalf("send tx offer to node 0: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		allAtHeight1 := true
		var headHash types.Hash
		hashesAgree := true
		for i, node := range nodes {
			h := node.Chain().HeadHeight()
			if h != 1 {
				allAtHeight1 = false
				break
			}
			if i == 0 {
				headHash = node.Chain().HeadHash()
			} else if node.Chain().HeadHash() != headHash {
				hashesAgree = false
			}
		}
		if allAtHeight1 && hashesAgree {
			break
		}
		if time.Now().After(deadline) {
			for i, node := range nodes {
				t.Logf("node %d: height=%d hash=%s", i, node.Chain().HeadHeight(), node.Chain().HeadHash())
			}
			t.Fatalf("nodes did not converge on height 1 with identical head hash within the deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Logf("all %d nodes converged on height 1, hash=%s", n, nodes[0].Chain().HeadHash())
}

func openIntegrationStore(t *testing.T, idx int) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("integration-test-key-32-bytes-p!"))
	key[31] = byte(idx) // distinct encryption key per node; irrelevant to correctness but avoids any accidental sharing assumption
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustSignIntegrationVote(t *testing.T, proposalID string) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	in := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID(proposalID),
			Commitment: types.Hash{1, 2, 3, 4},
		},
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}
