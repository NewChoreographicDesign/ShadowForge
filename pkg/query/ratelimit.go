package query

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Default per-remote-IP token bucket parameters. Generous enough for a
// real wallet polling several endpoints on a normal interval, restrictive
// enough that one caller can't cheaply hammer a node's Badger reads —
// this HTTP surface is new, unauthenticated, and (per Ignition's own
// guidance) may be reachable from the open internet, so it needs its own
// abuse defense rather than relying on anything upstream.
const (
	defaultRateLimit = 10 // sustained requests per second, per remote IP
	defaultBurst     = 20 // short burst allowance on top of the sustained rate

	// idleEntryTTL bounds how long a per-IP limiter is kept after its last
	// request before cleanup reclaims it — without this, every distinct
	// caller IP a public node ever sees would occupy memory forever.
	idleEntryTTL = 10 * time.Minute
)

// ipRateLimiter tracks one token-bucket limiter per remote IP and
// periodically forgets IPs that have gone quiet, so long-running public
// nodes don't leak memory over one caller per address seen.
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	limit   rate.Limit
	burst   int
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(requestsPerSecond float64, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		entries: make(map[string]*limiterEntry),
		limit:   rate.Limit(requestsPerSecond),
		burst:   burst,
	}
}

// allow reports whether a request from ip may proceed right now, creating
// a fresh bucket for an IP this limiter hasn't seen before.
func (rl *ipRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.entries[ip]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(rl.limit, rl.burst)}
		rl.entries[ip] = e
	}
	e.lastSeen = time.Now()
	rl.mu.Unlock()
	return e.limiter.Allow()
}

// cleanupLoop evicts limiter entries idle longer than idleEntryTTL, until
// ctx is done. Run as a background goroutine for the lifetime of a Server.
func (rl *ipRateLimiter) cleanupLoop(done <-chan struct{}) {
	ticker := time.NewTicker(idleEntryTTL)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			rl.mu.Lock()
			for ip, e := range rl.entries {
				if now.Sub(e.lastSeen) > idleEntryTTL {
					delete(rl.entries, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// rateLimitMiddleware rejects a request with 429 once its remote IP
// exceeds the configured rate, and otherwise passes it through unchanged.
func (rl *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r)
		if !rl.allow(ip) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
