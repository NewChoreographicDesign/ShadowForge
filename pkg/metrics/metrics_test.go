package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestGaugesReflectRealSetCalls proves the gauges genuinely track
// whatever a caller last reported, read back via testutil.ToFloat64 —
// the real collector value a Prometheus server's scrape would also see,
// not a mock.
func TestGaugesReflectRealSetCalls(t *testing.T) {
	SetChainHeight(42)
	if got := testutil.ToFloat64(chainHeight); got != 42 {
		t.Fatalf("expected chain height 42, got %v", got)
	}

	SetChainHeight(43)
	if got := testutil.ToFloat64(chainHeight); got != 43 {
		t.Fatalf("expected chain height to update to 43, got %v", got)
	}

	SetMempoolSize(7)
	if got := testutil.ToFloat64(mempoolSize); got != 7 {
		t.Fatalf("expected mempool size 7, got %v", got)
	}

	SetOnlineValidators(3)
	if got := testutil.ToFloat64(onlineValidators); got != 3 {
		t.Fatalf("expected online validators 3, got %v", got)
	}
}

// TestAddBlocksCommittedAccumulatesRealDeltas proves the counter really
// accumulates across multiple calls, and that a zero delta is a genuine
// no-op — never a spurious increment.
func TestAddBlocksCommittedAccumulatesRealDeltas(t *testing.T) {
	before := testutil.ToFloat64(blocksCommittedTotal)
	AddBlocksCommitted(5)
	AddBlocksCommitted(0)
	AddBlocksCommitted(2)
	after := testutil.ToFloat64(blocksCommittedTotal)
	if got := after - before; got != 7 {
		t.Fatalf("expected the counter to have advanced by exactly 7, got %v", got)
	}
}

// TestNodeInfoCarriesRealLabels proves the labeled gauge exposes the
// exact label values passed in.
func TestNodeInfoCarriesRealLabels(t *testing.T) {
	SetNodeInfo("sentinel", "deadbeef")
	if got := testutil.ToFloat64(nodeInfo.WithLabelValues("sentinel", "deadbeef")); got != 1 {
		t.Fatalf("expected a real labeled node_info sample of 1, got %v", got)
	}
}

// TestObserveQueryRequestCountsRealObservations proves the request
// counter accumulates per distinct (path, status) label pair
// independently, mirroring how pkg/query's own middleware calls it once
// per real HTTP request served.
func TestObserveQueryRequestCountsRealObservations(t *testing.T) {
	before := testutil.ToFloat64(queryRequestsTotal.WithLabelValues("/v1/status", "200"))
	ObserveQueryRequest("/v1/status", "200")
	ObserveQueryRequest("/v1/status", "200")
	after := testutil.ToFloat64(queryRequestsTotal.WithLabelValues("/v1/status", "200"))
	if got := after - before; got != 2 {
		t.Fatalf("expected exactly 2 new observations for this label pair, got %v", got)
	}

	// A distinct label pair is tracked independently, not merged into
	// the same series.
	ObserveQueryRequest("/v1/blocks/0", "404")
	if got := testutil.ToFloat64(queryRequestsTotal.WithLabelValues("/v1/blocks/0", "404")); got != 1 {
		t.Fatalf("expected a distinct label pair to have its own independent count, got %v", got)
	}
}
