// Package metrics is ShadowForge's real Prometheus instrumentation.
// Every collector here is wired to genuine, live node/network state —
// chain height, mempool size, online validator count, real pkg/query
// HTTP traffic — updated at the exact point each event happens (cmd/
// node's periodic sampler for the gauges, pkg/query's own request
// middleware for the counter). Nothing here is a synthetic, random, or
// placeholder value: a Prometheus server scraping cmd/node's real
// -metrics-listen endpoint (promhttp.Handler, registered against the
// same prometheus.DefaultRegisterer these collectors use) sees exactly
// what this process itself observed, plus the standard Go runtime/
// process collectors promauto's default registry always includes for
// free (go_goroutines, process_resident_memory_bytes, etc.).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	chainHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shadowforge_chain_height",
		Help: "Current real chain height this node has committed (chain.Chain.HeadHeight).",
	})

	blocksCommittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shadowforge_blocks_committed_total",
		Help: "Real blocks this process has observed committed to its own chain since it started (monotonic height increases only; resets to 0 on restart, so use rate()/increase(), not the raw value, across restarts).",
	})

	mempoolSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shadowforge_mempool_size",
		Help: "Current real number of transactions sitting in this node's mempool (tx.Mempool.Len).",
	})

	onlineValidators = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shadowforge_online_validators",
		Help: "Current real count of validator identities (any role) this node observes as online via heartbeat (validator.Node.OnlineValidatorCount).",
	})

	nodeInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shadowforge_node_info",
		Help: "Always 1; labels carry this node's own real static identity/role, for joining against the other gauges/counters in Prometheus or Grafana.",
	}, []string{"role", "identity"})

	queryRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shadowforge_query_requests_total",
		Help: "Real pkg/query HTTP requests this node has served, by request path and response status code.",
	}, []string{"path", "status"})
)

// SetChainHeight records this node's real, currently-committed chain
// height.
func SetChainHeight(h uint64) { chainHeight.Set(float64(h)) }

// AddBlocksCommitted credits n real, newly-observed committed blocks —
// callers are responsible for computing n as an actual, non-negative
// height delta since their own last observation (see cmd/node's
// metricsLoop, which mirrors chainStatusLoop's own "no baseline yet"
// sentinel so a restart never credits blocks committed in a prior
// process lifetime).
func AddBlocksCommitted(n uint64) {
	if n == 0 {
		return
	}
	blocksCommittedTotal.Add(float64(n))
}

// SetMempoolSize records this node's real, current mempool depth.
func SetMempoolSize(n int) { mempoolSize.Set(float64(n)) }

// SetOnlineValidators records this node's real, current count of
// heartbeat-online validator identities.
func SetOnlineValidators(n int) { onlineValidators.Set(float64(n)) }

// SetNodeInfo publishes this node's own real, static identity/role as a
// labeled gauge — called once at startup, not on every sample tick,
// since neither value ever changes for the life of the process.
func SetNodeInfo(role, identity string) {
	nodeInfo.WithLabelValues(role, identity).Set(1)
}

// ObserveQueryRequest records one real pkg/query HTTP request pkg/query's
// own logging middleware just finished serving.
func ObserveQueryRequest(path, status string) {
	queryRequestsTotal.WithLabelValues(path, status).Inc()
}
