package statuscheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeQueryServer serves a real /v1/status endpoint (same shape
// pkg/query itself serves) that can be toggled up/down by the test, so
// Monitor's real HTTP polling has something genuine to observe
// succeeding or failing.
func fakeQueryServer(t *testing.T, up *atomic.Bool, height *atomic.Uint64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			// A real connection that accepts but never answers usefully
			// isn't what we want here — closing without a response
			// simulates the down/unreachable case a real client sees.
			hj, ok := w.(http.Hijacker)
			if ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
					return
				}
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"height":     height.Load(),
			"head_hash":  strings.Repeat("00", 32),
			"genesis_ms": 0,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckOnceRecordsRealUpAndDownResults(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	var height atomic.Uint64
	height.Store(5)
	srv := fakeQueryServer(t, &up, &height)

	m, err := NewMonitor(Config{Nodes: []NodeConfig{{Name: "n1", QueryBase: srv.URL}}})
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}

	m.CheckOnce(context.Background(), time.Second)
	snap := m.Snapshot()
	if len(snap) != 1 || len(snap[0].Checks) != 1 {
		t.Fatalf("expected exactly 1 node with 1 check, got %+v", snap)
	}
	if !snap[0].Checks[0].Up || snap[0].Checks[0].Height != 5 {
		t.Fatalf("expected an up check at height 5, got %+v", snap[0].Checks[0])
	}
	if snap[0].UptimePercent != 100 {
		t.Fatalf("expected 100%% uptime after one up check, got %v", snap[0].UptimePercent)
	}

	up.Store(false)
	m.CheckOnce(context.Background(), 200*time.Millisecond)
	snap = m.Snapshot()
	if len(snap[0].Checks) != 2 {
		t.Fatalf("expected 2 checks after a second poll, got %d", len(snap[0].Checks))
	}
	if snap[0].Checks[1].Up {
		t.Fatalf("expected the second check to be recorded as down, got %+v", snap[0].Checks[1])
	}
	if snap[0].Checks[1].Error == "" {
		t.Fatalf("expected a real error message on the down check")
	}
	if got := snap[0].UptimePercent; got != 50 {
		t.Fatalf("expected 50%% uptime (1 of 2 checks up), got %v", got)
	}
	if snap[0].Latest.Up {
		t.Fatalf("expected Latest to reflect the most recent (down) check")
	}
}

func TestHistoryIsCappedAtMaxHistory(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	var height atomic.Uint64
	srv := fakeQueryServer(t, &up, &height)

	m, err := NewMonitor(Config{Nodes: []NodeConfig{{Name: "n1", QueryBase: srv.URL}}, MaxHistory: 3})
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}
	for i := 0; i < 10; i++ {
		height.Store(uint64(i))
		m.CheckOnce(context.Background(), time.Second)
	}
	snap := m.Snapshot()
	if len(snap[0].Checks) != 3 {
		t.Fatalf("expected history capped at 3, got %d", len(snap[0].Checks))
	}
	// The oldest checks should have been dropped, keeping the most
	// recent ones (heights 7, 8, 9 from the loop above).
	if snap[0].Checks[len(snap[0].Checks)-1].Height != 9 {
		t.Fatalf("expected the most recent check to be height 9, got %+v", snap[0].Checks)
	}
}

func TestPersistenceRoundTripsAcrossRestart(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	var height atomic.Uint64
	height.Store(42)
	srv := fakeQueryServer(t, &up, &height)

	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	m1, err := NewMonitor(Config{Nodes: []NodeConfig{{Name: "n1", QueryBase: srv.URL}}, PersistPath: path})
	if err != nil {
		t.Fatalf("new monitor 1: %v", err)
	}
	m1.CheckOnce(context.Background(), time.Second)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected a persisted history file to exist: %v", err)
	}

	// A fresh Monitor over the same path — the real "process restart"
	// scenario a status page must survive — should load that same real
	// check right back, even against a node it hasn't polled itself yet.
	m2, err := NewMonitor(Config{Nodes: []NodeConfig{{Name: "n1", QueryBase: srv.URL}}, PersistPath: path})
	if err != nil {
		t.Fatalf("new monitor 2: %v", err)
	}
	snap := m2.Snapshot()
	if len(snap[0].Checks) != 1 || snap[0].Checks[0].Height != 42 {
		t.Fatalf("expected the persisted check to survive a restart, got %+v", snap)
	}
}

func TestNewMonitorRejectsEmptyAndDuplicateNodes(t *testing.T) {
	if _, err := NewMonitor(Config{}); err == nil {
		t.Fatalf("expected an error with zero configured nodes")
	}
	_, err := NewMonitor(Config{Nodes: []NodeConfig{
		{Name: "dup", QueryBase: "http://a"},
		{Name: "dup", QueryBase: "http://b"},
	}})
	if err == nil {
		t.Fatalf("expected an error on duplicate node names")
	}
}

func TestRunPerformsAnImmediateCheckBeforeFirstTick(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	var height atomic.Uint64
	srv := fakeQueryServer(t, &up, &height)

	m, err := NewMonitor(Config{Nodes: []NodeConfig{{Name: "n1", QueryBase: srv.URL}}})
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go m.Run(ctx, time.Hour, time.Second) // interval far longer than the test
	defer cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.Snapshot()[0].Checks) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected Run to perform an immediate check without waiting a full interval")
}
