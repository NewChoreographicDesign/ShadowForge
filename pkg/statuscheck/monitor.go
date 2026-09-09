// Package statuscheck is a real, minimal uptime monitor for one or more
// ShadowForge nodes' pkg/query APIs — the server-side half of a status
// page (cmd/statuspage): unlike the block explorer (pkg/query's own
// browser-direct-fetch design, pkg/query's own doc), uptime history has
// to be observed continuously by something that keeps running whether
// or not anyone has the page open, so this package polls, records, and
// persists real check results rather than only ever showing "right now."
package statuscheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
)

// Check is one real, timestamped poll of a node's /v1/status.
type Check struct {
	At time.Time `json:"at"`
	Up bool      `json:"up"`
	// Height is only meaningful when Up is true — deliberately no
	// omitempty: a real node genuinely at height 0 (fresh genesis) must
	// still serialize as 0, not silently vanish from the response the
	// way omitempty would (a real bug an earlier version of this struct
	// had, caught by the frontend rendering "height undefined").
	Height    uint64 `json:"height"`
	HeadHash  string `json:"head_hash,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// NodeConfig names one node to monitor.
type NodeConfig struct {
	// Name is a short, human label for this node (e.g. "validator1") —
	// shown on the status page and used as this node's key in the
	// persisted history file.
	Name string
	// QueryBase is its pkg/query API base URL (e.g. "http://127.0.0.1:8081").
	QueryBase string
}

type nodeState struct {
	cfg    NodeConfig
	checks []Check
}

// Config configures a Monitor.
type Config struct {
	Nodes []NodeConfig
	// MaxHistory caps how many recent checks are retained per node
	// (oldest dropped first). <= 0 defaults to 500.
	MaxHistory int
	// PersistPath, if non-empty, is a JSON file this Monitor loads its
	// starting history from (if it exists) and rewrites after every
	// check cycle — a real, restart-safe snapshot, not an ever-growing
	// log: it always holds exactly the current, capped in-memory
	// history, nothing more.
	PersistPath string
	// Logf receives operational log lines (persistence errors). Defaults
	// to a no-op.
	Logf func(format string, args ...any)
}

// Monitor polls a fixed set of nodes' real /v1/status on an interval and
// keeps a capped, optionally-persisted history of the results.
type Monitor struct {
	mu          sync.Mutex
	nodes       []*nodeState
	maxHistory  int
	persistPath string
	logf        func(format string, args ...any)
}

// NewMonitor builds a Monitor for cfg.Nodes, loading any existing
// persisted history from cfg.PersistPath first.
func NewMonitor(cfg Config) (*Monitor, error) {
	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("statuscheck: at least one node is required")
	}
	maxHistory := cfg.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 500
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	m := &Monitor{maxHistory: maxHistory, persistPath: cfg.PersistPath, logf: logf}
	seen := make(map[string]bool, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		if seen[n.Name] {
			return nil, fmt.Errorf("statuscheck: duplicate node name %q", n.Name)
		}
		seen[n.Name] = true
		m.nodes = append(m.nodes, &nodeState{cfg: n})
	}
	if cfg.PersistPath != "" {
		if err := m.load(); err != nil {
			return nil, fmt.Errorf("statuscheck: load persisted history: %w", err)
		}
	}
	return m, nil
}

// CheckOnce polls every configured node's real /v1/status exactly once,
// records the result (success or failure — a real, disclosed timeout or
// connection error is itself a genuine "down" data point, never
// silently dropped), and persists the resulting history if PersistPath
// is set.
func (m *Monitor) CheckOnce(ctx context.Context, timeout time.Duration) {
	now := time.Now()
	for _, ns := range m.nodes {
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		hc := &http.Client{Timeout: timeout}
		cl := queryclient.NewWithClient(ns.cfg.QueryBase, hc)
		status, err := cl.Status(checkCtx)
		cancel()
		latency := time.Since(start).Milliseconds()

		c := Check{At: now, LatencyMs: latency}
		if err != nil {
			c.Up = false
			c.Error = err.Error()
		} else {
			c.Up = true
			c.Height = status.Height
			c.HeadHash = status.HeadHash.String()
		}

		m.mu.Lock()
		ns.checks = append(ns.checks, c)
		if len(ns.checks) > m.maxHistory {
			ns.checks = ns.checks[len(ns.checks)-m.maxHistory:]
		}
		m.mu.Unlock()
	}
	if m.persistPath != "" {
		if err := m.save(); err != nil {
			m.logf("statuscheck: persist history: %v", err)
		}
	}
}

// Run calls CheckOnce immediately (so a fresh page load has real data
// without waiting a full interval) and then on every tick, until ctx is
// done.
func (m *Monitor) Run(ctx context.Context, interval, timeout time.Duration) {
	m.CheckOnce(ctx, timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckOnce(ctx, timeout)
		}
	}
}

// NodeSnapshot is one node's real, current monitoring state — a copy
// safe for a caller (e.g. an HTTP handler) to hold onto and render
// without racing further Monitor updates.
type NodeSnapshot struct {
	Name          string  `json:"name"`
	QueryBase     string  `json:"query_base"`
	Checks        []Check `json:"checks"`
	UptimePercent float64 `json:"uptime_percent"`
	Latest        *Check  `json:"latest,omitempty"`
}

// Snapshot returns a consistent, point-in-time copy of every configured
// node's real monitoring state, in configured order.
func (m *Monitor) Snapshot() []NodeSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]NodeSnapshot, 0, len(m.nodes))
	for _, ns := range m.nodes {
		checksCopy := make([]Check, len(ns.checks))
		copy(checksCopy, ns.checks)
		var latest *Check
		if len(checksCopy) > 0 {
			l := checksCopy[len(checksCopy)-1]
			latest = &l
		}
		out = append(out, NodeSnapshot{
			Name:          ns.cfg.Name,
			QueryBase:     ns.cfg.QueryBase,
			Checks:        checksCopy,
			UptimePercent: uptimePercent(checksCopy),
			Latest:        latest,
		})
	}
	return out
}

// uptimePercent is the real fraction of retained checks that were up —
// 0 (not unknown/100) for a node with no checks yet, since "no data"
// and "0% uptime" are both honestly "nothing to report as up" for
// display purposes here.
func uptimePercent(checks []Check) float64 {
	if len(checks) == 0 {
		return 0
	}
	up := 0
	for _, c := range checks {
		if c.Up {
			up++
		}
	}
	return 100 * float64(up) / float64(len(checks))
}

// persistedState is PersistPath's real on-disk shape: the complete,
// current (already-capped) history per node name, nothing else.
type persistedState struct {
	Nodes map[string][]Check `json:"nodes"`
}

func (m *Monitor) save() error {
	m.mu.Lock()
	state := persistedState{Nodes: make(map[string][]Check, len(m.nodes))}
	for _, ns := range m.nodes {
		checksCopy := make([]Check, len(ns.checks))
		copy(checksCopy, ns.checks)
		state.Nodes[ns.cfg.Name] = checksCopy
	}
	m.mu.Unlock()

	enc, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := m.persistPath + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.persistPath)
}

func (m *Monitor) load() error {
	raw, err := os.ReadFile(m.persistPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("parse %s: %w", m.persistPath, err)
	}
	for _, ns := range m.nodes {
		if checks, ok := state.Nodes[ns.cfg.Name]; ok {
			if len(checks) > m.maxHistory {
				checks = checks[len(checks)-m.maxHistory:]
			}
			ns.checks = checks
		}
	}
	return nil
}
