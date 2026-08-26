package consensus_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func nftID(b byte) types.NFTID {
	var id types.NFTID
	id[0] = b
	return id
}

func countOccurrences(items []types.QueueItem, id types.NFTID) int {
	n := 0
	for _, it := range items {
		if it.NFT == id {
			n++
		}
	}
	return n
}

// TestInsertValidatorScattersFivePositions covers spec 5.4.1 / 19.2 and the
// plain-language framing in section 1 ("New joiners are inserted at
// several 'unfair' positions"): a fresh joiner ends up with 5 queue
// entries, not 1.
func TestInsertValidatorScattersFivePositions(t *testing.T) {
	r := consensus.NewRevolver()
	a, b := nftID(0xA), nftID(0xB)
	r.InsertValidator(types.QueueItem{NFT: a, TP: consensus.LowTPThreshold})
	r.InsertValidator(types.QueueItem{NFT: b, TP: consensus.LowTPThreshold})

	items := r.Snapshot()
	if len(items) != 10 {
		t.Fatalf("expected 10 total queue entries (5 per validator), got %d", len(items))
	}
	if n := countOccurrences(items, a); n != 5 {
		t.Fatalf("expected validator A to occupy 5 slots, got %d", n)
	}
	if n := countOccurrences(items, b); n != 5 {
		t.Fatalf("expected validator B to occupy 5 slots, got %d", n)
	}
}

// TestInsertValidatorDuplicateIgnored is the Testing Matrix (section 20)
// "Revolver: ... duplicate ignored" requirement.
func TestInsertValidatorDuplicateIgnored(t *testing.T) {
	r := consensus.NewRevolver()
	a := nftID(0xA)
	r.InsertValidator(types.QueueItem{NFT: a, TP: consensus.LowTPThreshold})
	firstLen := r.Len()
	r.InsertValidator(types.QueueItem{NFT: a, TP: consensus.LowTPThreshold})
	if r.Len() != firstLen {
		t.Fatalf("duplicate join request must be a no-op: len went from %d to %d", firstLen, r.Len())
	}
}

func TestPopFrontPushBackFIFOOrder(t *testing.T) {
	r := consensus.NewRevolver()
	a, b, c := nftID(1), nftID(2), nftID(3)
	// Push directly to avoid the scatter algorithm's fan-out, isolating
	// PopFront/PushBack ordering itself.
	r.PushBack(types.QueueItem{NFT: a})
	r.PushBack(types.QueueItem{NFT: b})
	r.PushBack(types.QueueItem{NFT: c})

	item, ok := r.PopFront()
	if !ok || item.NFT != a {
		t.Fatalf("expected to pop A first, got %+v ok=%v", item, ok)
	}
	r.PushBack(item) // successful stage work: requeue at the back
	item, ok = r.PopFront()
	if !ok || item.NFT != b {
		t.Fatalf("expected to pop B second, got %+v ok=%v", item, ok)
	}
}

func TestValidatorsPerStageAdaptiveWidth(t *testing.T) {
	cases := []struct {
		online int
		want   int
	}{
		{0, 1}, {200, 1}, {201, 2}, {300, 2}, {301, 3}, {1000, 3},
	}
	for _, c := range cases {
		if got := consensus.ValidatorsPerStage(c.online); got != c.want {
			t.Errorf("ValidatorsPerStage(%d) = %d, want %d", c.online, got, c.want)
		}
	}
}

func TestSentinelsNeededThreshold(t *testing.T) {
	cases := []struct {
		online int
		want   bool
	}{
		{0, true}, {9, true}, {10, false}, {11, false},
	}
	for _, c := range cases {
		if got := consensus.SentinelsNeeded(c.online); got != c.want {
			t.Errorf("SentinelsNeeded(%d) = %v, want %v", c.online, got, c.want)
		}
	}
}

func TestIsOfflineAfterThreeMissedBeats(t *testing.T) {
	now := time.Now()
	lastBeat := now.Add(-29 * time.Second)
	if consensus.IsOffline(lastBeat, now) {
		t.Fatalf("29s since last beat should not yet be offline")
	}
	lastBeat = now.Add(-31 * time.Second)
	if !consensus.IsOffline(lastBeat, now) {
		t.Fatalf("31s since last beat (>3 missed 10s beats) should be offline")
	}
}

func TestApplyCooldownRemovesFromQueue(t *testing.T) {
	r := consensus.NewRevolver()
	a := nftID(0xA)
	r.PushBack(types.QueueItem{NFT: a})
	r.PushBack(types.QueueItem{NFT: a})
	r.ApplyCooldown(a, time.Now())
	if n := countOccurrences(r.Snapshot(), a); n != 0 {
		t.Fatalf("expected all entries for a cooldown'd validator to be removed, found %d", n)
	}
}

func TestOnlineExcludesCooldown(t *testing.T) {
	r := consensus.NewRevolver()
	now := time.Now()
	active := nftID(1)
	cooling := nftID(2)
	r.PushBack(types.QueueItem{NFT: active})
	r.PushBack(types.QueueItem{NFT: cooling, CooldownUntil: now.Add(time.Hour).UnixMilli()})
	if got := r.Online(now); got != 1 {
		t.Fatalf("expected 1 online validator excluding cooldown, got %d", got)
	}
}

func TestLowTPSybilBrakeDefersInsert(t *testing.T) {
	r := consensus.NewRevolver()
	now := time.Now()
	low := types.QueueItem{NFT: nftID(1), TP: consensus.LowTPThreshold - 1}
	r.RequestJoin(low, now)
	if r.Len() != 0 {
		t.Fatalf("low-TP joiner should be deferred, not inserted immediately; len=%d", r.Len())
	}
	// Not yet eligible.
	r.ReleaseEligiblePending(now.Add(consensus.LowTPDelay/2), func(id types.NFTID) (types.QueueItem, bool) {
		return low, true
	})
	if r.Len() != 0 {
		t.Fatalf("low-TP joiner should still be deferred before the brake delay elapses; len=%d", r.Len())
	}
	// Now eligible.
	r.ReleaseEligiblePending(now.Add(consensus.LowTPDelay+time.Millisecond), func(id types.NFTID) (types.QueueItem, bool) {
		return low, true
	})
	if r.Len() == 0 {
		t.Fatalf("expected low-TP joiner to be inserted once the brake delay elapses")
	}
}

func TestHighTPJoinsImmediately(t *testing.T) {
	r := consensus.NewRevolver()
	high := types.QueueItem{NFT: nftID(1), TP: consensus.LowTPThreshold}
	r.RequestJoin(high, time.Now())
	if r.Len() == 0 {
		t.Fatalf("expected a high-TP joiner to be inserted immediately")
	}
}
