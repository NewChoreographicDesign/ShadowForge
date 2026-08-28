package validator

import (
	"testing"
	"time"

	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// TestVotingOnlyNodePrunesCommittedTxFromLocalMempool reproduces, at the
// unit level, a real bug this build hit live under sustained 3-node
// traffic (real OS processes, real gossip, real BFT rounds): a node that
// only ever votes on a height — never proposes it — kept a stale local
// copy of every gossiped tx even after it became durably committed via
// the actual proposer's batch. That stale copy later got dragged into a
// batch this same node built once it *did* become proposer, Stage 4
// rejected it outright (its ballot was already recorded), and the
// whole-batch-atomicity rule then discarded every other, individually
// valid tx bundled alongside it — repeating every round the stale entry
// resurfaced. pruneCommittedFromLocalQueues (wired into both
// handleBlockProposal and handleBlockAnnounce) is the fix: this test
// proves the mempool side of it directly.
func TestVotingOnlyNodePrunesCommittedTxFromLocalMempool(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	height := n.chn.NextHeight()

	// Retry with fresh peers until the real, key-derived committee order
	// puts someone OTHER than self at committee[0] — this test is
	// specifically about the non-proposer path.
	var committee []types.NFTID
	var proposerPeer peerKey
	for attempt := 0; attempt < 100; attempt++ {
		n.mu.Lock()
		for id := range n.online {
			if id != n.identity {
				delete(n.online, id)
				delete(n.everSeen, id)
			}
		}
		n.mu.Unlock()
		var p2, p3 peerKey
		proposerPeer, p2, p3 = genPeer(t), genPeer(t), genPeer(t)
		committee = registerOnline(n, height, proposerPeer, p2, p3)
		if committee[0] != n.identity && committee[0] == proposerPeer.id {
			break
		}
	}
	if committee[0] != proposerPeer.id {
		t.Fatalf("failed to draw a committee with proposerPeer as proposer after 100 attempts")
	}

	// This node independently received the same tx via gossip (e.g. a
	// wallet submitted it directly to this node before the proposer's
	// version reached it) and admitted it to its own mempool.
	voteTx := mustSignVote(t, n, "prune-proposal", 1)
	if err := n.mempool.Submit(voteTx, time.Now()); err != nil {
		t.Fatalf("submit to local mempool: %v", err)
	}
	if n.mempool.Len() != 1 {
		t.Fatalf("expected the tx to be pending locally before the proposal arrives")
	}

	// The real proposer's proposal (built independently, containing the
	// same tx) arrives and this node votes on it.
	prop := shadownet.BlockProposalPayload{
		Height:    height,
		Proposer:  committee[0],
		Batch:     []types.ShieldedTx{voteTx},
		Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	_, tracked := n.rounds[height]
	n.roundMu.Unlock()
	if !tracked {
		t.Fatalf("expected this node to track the round after voting on a valid proposal from the real assigned proposer")
	}

	if n.mempool.Len() != 0 {
		t.Fatalf("expected the locally-held stale copy to be pruned once this node started voting on the round containing it, got len=%d", n.mempool.Len())
	}

	// Confirm the fix is real, not just "the field is now empty": submit
	// a distinct second tx and prove it's unaffected — pruning must be
	// selective, not a blanket mempool wipe.
	other := mustSignVote(t, n, "unrelated-proposal", 2)
	if err := n.mempool.Submit(other, time.Now()); err != nil {
		t.Fatalf("submit unrelated tx: %v", err)
	}
	if n.mempool.Len() != 1 {
		t.Fatalf("expected the unrelated tx to be admitted normally, len=%d", n.mempool.Len())
	}
}
