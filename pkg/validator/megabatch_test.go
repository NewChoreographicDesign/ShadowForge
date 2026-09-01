package validator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// TestMegabatchPartRealWireReassembly proves shadownet.MsgMegabatchPart
// chunking and reassembly work over a real libp2p connection between two
// independent Node instances — not a direct method-call shortcut into
// another node's internals. The sender declares a real outage, enqueues
// enough real backlog to force more than one MaxMegabatchPartBytes-sized
// chunk, and calls the real dual-track buildProposalBatch path, which
// broadcasts the full pre-trim megabatch over the wire; the receiver
// (which never starts its own round loop — a passive observer, mirroring
// catchup_test.go's node B) reassembles it purely from real
// MsgMegabatchPart envelopes and records it under ReassembledMegabatch.
func TestMegabatchPartRealWireReassembly(t *testing.T) {
	genesisMs := time.Now().UnixMilli()
	sender := newTestNode(t, time.Minute, genesisMs)
	receiver := newTestNode(t, time.Minute, genesisMs)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	addrs := shadownet.FullAddr(receiver.net.Host)
	if len(addrs) == 0 {
		t.Fatalf("receiver has no listen addresses")
	}
	connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
	defer connectCancel()
	if err := shadownet.Connect(connectCtx, sender.net.Host, addrs[0]); err != nil {
		t.Fatalf("connect sender -> receiver: %v", err)
	}

	sender.outage.Declare()

	// 6 real, signed txs padded to 20 KiB apiece (120 KiB total) — well
	// past MaxMegabatchPartBytes (64 KiB), so the sender genuinely has to
	// split this into multiple real wire chunks, not just one.
	const backlogCount = 6
	const paddingLen = 20 * 1024
	now := time.Now()
	var built []types.ShieldedTx
	for i := 0; i < backlogCount; i++ {
		bt := sizedVoteTx(t, sender, fmt.Sprintf("megabatch-backlog-%d", i), byte(i+1), paddingLen)
		if err := sender.outage.Enqueue(bt, now); err != nil {
			t.Fatalf("enqueue backlog tx %d: %v", i, err)
		}
		built = append(built, bt)
	}

	wantSize, err := marshaledSize(built)
	if err != nil {
		t.Fatalf("measure want size: %v", err)
	}
	if wantSize <= shadownet.MaxMegabatchPartBytes {
		t.Fatalf("test setup bug: backlog (%d bytes) must exceed MaxMegabatchPartBytes (%d) to actually exercise multi-part chunking", wantSize, shadownet.MaxMegabatchPartBytes)
	}

	height := sender.chn.NextHeight()
	sender.buildProposalBatch(true) // real dual-track path: drains the backlog and broadcasts real MegabatchPart chunks

	deadline := time.Now().Add(5 * time.Second)
	var got []types.ShieldedTx
	for {
		if b, ok := receiver.ReassembledMegabatch(height); ok {
			got = b
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the receiver to reassemble the real megabatch")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(got) != backlogCount {
		t.Fatalf("expected %d reassembled tx(es), got %d", backlogCount, len(got))
	}
	for i, want := range built {
		if got[i].TxID != want.TxID {
			t.Fatalf("tx %d mismatch: got TxID %s, want %s", i, got[i].TxID, want.TxID)
		}
	}

	// The sender's own backlog must be fully drained — BuildMegabatch
	// took every entry (backlogCount is well under
	// MegabatchMultiplier*maxBatchSize), and none of it was lost.
	if depth := sender.outage.BacklogDepth(); depth != 0 {
		t.Fatalf("expected the sender's backlog to be fully drained, got depth %d", depth)
	}
}

// TestMegabatchPartRejectsOversizedPartCount proves handleMegabatchPart's
// own defense-in-depth: a peer claiming a PartCount beyond
// shadownet.MaxMegabatchParts is rejected outright rather than driving an
// unbounded allocation, and never contaminates a later, honest
// announcement for the same (sender, height) key.
func TestMegabatchPartRejectsOversizedPartCount(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	badSender := peer.ID("attacker-peer")

	n.handleMegabatchPart(badSender, shadownet.MegabatchPartPayload{
		Height: 1, PartIndex: 0, PartCount: shadownet.MaxMegabatchParts + 1, Data: []byte("x"),
	})
	if _, ok := n.ReassembledMegabatch(1); ok {
		t.Fatalf("expected an oversized PartCount claim to never reassemble into anything")
	}
	n.megabatchMu.Lock()
	pending := len(n.megabatchRecv)
	n.megabatchMu.Unlock()
	if pending != 0 {
		t.Fatalf("expected the rejected claim to leave no partial assembly state behind, got %d pending", pending)
	}
}
