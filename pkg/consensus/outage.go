// outage.go implements the linear outage recovery pipeline from spec 5.6
// and Outage Handling.pdf:
//
//	Detect -> pause live admission, backlog incoming txs -> activate
//	sentinels if needed -> containers flip to internal mode (pkg/container)
//	-> on recovery, build megabatches (10x normal batch) -> dual-track
//	(Track A live, Track B backlog, DualTrack=true on backlog blocks) ->
//	clear OutageFlag once backlog is below threshold and one clean
//	dual-track cycle has committed.
package consensus

import (
	"sync"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// MegabatchMultiplier is the "10x normal batch size" from spec 5.6.
const MegabatchMultiplier = 10

// OutageThresholds are the governance-configurable knobs for outage
// detection and recovery (spec 5.6, 22).
type OutageThresholds struct {
	// MissingHeartbeatFraction: outage is declared when more than this
	// fraction of the last-known online set is missing heartbeats.
	MissingHeartbeatFraction float64
	// BacklogClearThreshold: OutageFlag may only clear once backlog depth
	// is at or below this many transactions.
	BacklogClearThreshold int
}

func DefaultOutageThresholds() OutageThresholds {
	return OutageThresholds{MissingHeartbeatFraction: 0.5, BacklogClearThreshold: 0}
}

// OutageController is the node-local state machine driving the recovery
// pipeline. It owns the on-disk-backed backlog queue (spec 5.6: "Incoming
// user transactions go to an on-disk backlog queue"); this in-memory
// implementation is wrapped by pkg/state for actual persistence.
type OutageController struct {
	mu          sync.Mutex
	thresholds  OutageThresholds
	flag        bool
	backlog     []types.ShieldedTx
	cleanCycles int
}

func NewOutageController(t OutageThresholds) *OutageController {
	return &OutageController{thresholds: t}
}

// DetectOutage implements spec 5.6's detection condition: "heartbeats are
// missing from more than 50 percent of the last-known online set."
func (o *OutageController) DetectOutage(lastKnownOnline, missing int) bool {
	if lastKnownOnline <= 0 {
		return false
	}
	return float64(missing)/float64(lastKnownOnline) > o.thresholds.MissingHeartbeatFraction
}

// Declare raises OutageFlag and pauses live batch admission; callers must
// route new incoming transactions to Enqueue instead of the live mempool
// while Active() is true.
func (o *OutageController) Declare() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.flag = true
	o.cleanCycles = 0
}

func (o *OutageController) Active() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.flag
}

// Enqueue adds an incoming transaction to the backlog queue. Wallets may
// keep signing offline (spec 5.6); those signed transactions arrive here
// once connectivity is restored enough to submit.
func (o *OutageController) Enqueue(tx types.ShieldedTx) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.backlog = append(o.backlog, tx)
}

func (o *OutageController) BacklogDepth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.backlog)
}

// BuildMegabatch drains up to MegabatchMultiplier*normalBatchSize
// transactions from the backlog as Track B of a dual-track recovery batch
// (spec 5.6: "build megabatches of 10x normal batch size from the
// backlog... Track B drains backlog").
func (o *OutageController) BuildMegabatch(normalBatchSize int) []types.ShieldedTx {
	o.mu.Lock()
	defer o.mu.Unlock()
	max := normalBatchSize * MegabatchMultiplier
	if max <= 0 || len(o.backlog) == 0 {
		return nil
	}
	n := len(o.backlog)
	if n > max {
		n = max
	}
	batch := make([]types.ShieldedTx, n)
	copy(batch, o.backlog[:n])
	o.backlog = o.backlog[n:]
	return batch
}

// RecordCleanDualTrackCycle marks that one dual-track batch (Track A live +
// Track B backlog) committed successfully. spec 5.6 requires "one clean
// dual-track cycle has committed" before OutageFlag may clear.
func (o *OutageController) RecordCleanDualTrackCycle() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cleanCycles++
}

// MaybeClear clears OutageFlag once backlog depth is at or below threshold
// and at least one clean dual-track cycle has committed, and reports
// whether it did so.
func (o *OutageController) MaybeClear() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.flag {
		return false
	}
	if len(o.backlog) > o.thresholds.BacklogClearThreshold {
		return false
	}
	if o.cleanCycles < 1 {
		return false
	}
	o.flag = false
	return true
}
