package validator_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/validator"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// lossyHost wraps a real libp2p host.Host and injects real, bounded
// network jitter and packet loss into every outbound stream this node
// opens. pkg/net.Node.Send and Broadcast — the only path every wire
// message (heartbeat, TxOffer, BlockProposal, StageVote, BlockAnnounce,
// MegabatchPart, BlockRequest/Response) actually travels — both go
// through host.Host.NewStream, so overriding just that one method faults
// the entire real message layer without touching how streams are read,
// how the initial mesh is dialed (Connect goes straight to the embedded
// Host, unaffected — exactly as a real lossy link still completes a TCP
// handshake but drops or delays individual application messages), or any
// production code path: this type exists only in this test file.
type lossyHost struct {
	host.Host

	mu        sync.Mutex
	rng       *rand.Rand
	lossProb  float64
	maxJitter time.Duration
}

func newLossyHost(h host.Host, seed int64, lossProb float64, maxJitter time.Duration) *lossyHost {
	return &lossyHost{Host: h, rng: rand.New(rand.NewSource(seed)), lossProb: lossProb, maxJitter: maxJitter}
}

func (h *lossyHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	h.mu.Lock()
	drop := h.rng.Float64() < h.lossProb
	var jitter time.Duration
	if h.maxJitter > 0 {
		jitter = time.Duration(h.rng.Int63n(int64(h.maxJitter) + 1))
	}
	h.mu.Unlock()

	if drop {
		return nil, fmt.Errorf("lossyHost: simulated packet loss opening stream to %s", p)
	}
	if jitter > 0 {
		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return h.Host.NewStream(ctx, p, pids...)
}

// TestFourNodesConvergeUnderJitterAndPacketLoss extends
// TestFourNodesConvergeOnSameChain's real 4-node, real-libp2p-network
// topology with a real, adversarial network condition: every outbound
// message any node sends has a genuine 15% chance of being dropped
// outright and, when it isn't, a real 0-120ms random delivery delay —
// applied independently per message, per node, exactly the way jitter
// and packet loss behave on a real, imperfect network (spec 18.6's
// hardening phase names this explicitly). Consensus liveness here comes
// entirely from the real, already-shipped mechanisms this induces:
// roundLoop's periodic re-proposal after sweepTimeouts rolls back an
// expired round (pkg/validator/consensus.go), gossip-based redundancy
// across a full mesh so a single dropped link rarely isolates a
// committee member, and BFT quorum only requiring a supermajority of an
// assigned committee rather than every node. The test proves the same
// four-node convergence guarantee as the clean-network test still holds
// — just, realistically, slower.
func TestFourNodesConvergeUnderJitterAndPacketLoss(t *testing.T) {
	const n = 4
	const lossProb = 0.15
	const maxJitter = 120 * time.Millisecond
	genesisMs := time.Now().UnixMilli()

	cfg := validator.Config{
		BatchInterval:     100 * time.Millisecond,
		RoundTimeout:      6 * time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
		OnlineTimeout:     5 * time.Second,
		Genesis:           consensus.GenesisTime(genesisMs),
	}

	eligSys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("eligibility zk setup: %v", err)
	}

	nodes := make([]*validator.Node, n)
	hosts := make([]host.Host, n)
	stores := make([]*state.Store, n)
	for i := 0; i < n; i++ {
		rawHost, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
		if err != nil {
			t.Fatalf("node %d: new host: %v", i, err)
		}
		t.Cleanup(func() { _ = rawHost.Close() })
		// A distinct seed per node keeps each node's fault pattern
		// independent, exactly like independent links on a real network
		// rather than a single global on/off switch.
		h := newLossyHost(rawHost, int64(1000+i), lossProb, maxJitter)
		hosts[i] = h

		store := openIntegrationStore(t, 100+i)
		stores[i] = store
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
		nodes[i] = validator.NewNode(cfg, h, nil, store, tree, chn, nil, v, nil, nil, eligSys, nil, nil, nil, mempool, pk, sk, false, logf)
	}

	// Full mesh, dialed on the wrapped hosts — Connect() forwards straight
	// to the embedded real host (lossyHost only overrides NewStream), so
	// the initial connections themselves are reliable, exactly like a
	// real TCP handshake succeeding even on a lossy link.
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

	// Give the heartbeat mesh longer to converge than the clean-network
	// test — under real packet loss, individual heartbeats are dropped,
	// so more rounds are needed before every node's onlineSet agrees.
	time.Sleep(40 * cfg.HeartbeatInterval)

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

	voteTx := mustSignIntegrationVote(t, nodes, stores, eligSys, "jitter-loss-proposal")
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: voteTx})
	if err != nil {
		t.Fatalf("build tx offer: %v", err)
	}
	// The wallet's own send to node 0 is itself real user traffic over a
	// real network — it can be dropped too. Retry it the way a real
	// wallet's own submission logic would, rather than depending on a
	// single lucky send landing before the offer's TTL (pkg/tx.TxTTL)
	// expires.
	offerDeadline := time.Now().Add(3 * time.Second)
	for {
		if err := walletNode.Send(ctx, hosts[0].ID(), env); err == nil {
			break
		} else if time.Now().After(offerDeadline) {
			t.Fatalf("send tx offer to node 0: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A much longer deadline than the clean-network test: real jitter and
	// packet loss mean some rounds will time out and retry
	// (sweepTimeouts + roundLoop's next tick), not fail outright.
	//
	// Phase 2 independent audit finding: this deadline was widened from
	// 90s after fixing a real BFT safety bug (consensus.BFTQuorumMet used
	// to accept a simple majority, which a single equivocating validator
	// could exploit to finalize two conflicting blocks at the same height
	// — see TestBFTQuorumUnsafeAgainstClaimedFaultTolerance). The fix
	// requires a genuine >2/3 supermajority instead of a bare majority,
	// mathematically necessary for the safety this codebase already
	// claims (BFTFaultTolerance's "tolerates up to one third faulty
	// nodes"), but it also means more of a committee's votes must survive
	// the same 15%-per-message packet loss before a round finalizes, so
	// real convergence under this test's adversarial conditions genuinely
	// needs a few more retries on most runs — a real, expected
	// safety/liveness trade-off, not a regression this test should paper
	// over by staying lenient on quorum.
	//
	// The stricter quorum also surfaced (and this pass fixed) a real,
	// structural — not merely probabilistic — stall: tryAdoptBlockLocked's
	// committee recomputation used to exclude this node's own identity
	// unconditionally, which is correct for genuine multi-block catch-up
	// (see that function's own doc) but wrong for an ordinary single-block
	// announce, where self was almost certainly a live committee member.
	// In a small committee (e.g. this test's 2-real-validator core), that
	// miscount could shrink the recomputed committee below
	// consensus.MinCommitteeSize, permanently rejecting an
	// already-quorate block with "0/0 votes" on every retry, no matter how
	// many — the run-to-run flakiness observed while diagnosing this test
	// traced back to exactly that, not to ordinary packet-loss bad luck.
	deadline := time.Now().Add(120 * time.Second)
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
			t.Fatalf("nodes did not converge on height 1 with identical head hash within the deadline, under %.0f%% simulated packet loss and up to %s jitter", lossProb*100, maxJitter)
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("all %d nodes converged on height 1 under %.0f%% packet loss / up to %s jitter, hash=%s", n, lossProb*100, maxJitter, nodes[0].Chain().HeadHash())
}
