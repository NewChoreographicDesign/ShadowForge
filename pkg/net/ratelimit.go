package net

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// bucket is a simple token bucket: capacity tokens, refilling at
// refillRate tokens/second.
type bucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
}

func newBucket(capacity, refillRate float64, now time.Time) *bucket {
	return &bucket{tokens: capacity, capacity: capacity, refillRate: refillRate, lastRefill: now}
}

func (b *bucket) take(now time.Time) bool {
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.refillRate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastRefill = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimiterConfig sets the token-bucket capacity/refill and drop
// cooldown. Defaults mirror spec 6: "per-peer token bucket on Heartbeat and
// TxOffer. Exceeding the bucket drops the peer for a cooldown. This is the
// first DoS brake."
type RateLimiterConfig struct {
	Capacity   float64
	RefillRate float64 // tokens/second
	Cooldown   time.Duration
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{Capacity: 20, RefillRate: 5, Cooldown: 30 * time.Second}
}

// RateLimiter enforces a per-peer, per-message-type token bucket and drops
// (cooldowns) a peer that exceeds it.
type RateLimiter struct {
	cfg RateLimiterConfig

	mu           sync.Mutex
	buckets      map[peer.ID]map[MessageType]*bucket
	droppedUntil map[peer.ID]time.Time
}

func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	return &RateLimiter{
		cfg:          cfg,
		buckets:      map[peer.ID]map[MessageType]*bucket{},
		droppedUntil: map[peer.ID]time.Time{},
	}
}

// Allow reports whether a message of type t from p should be processed. It
// applies to the message types spec 6 names as rate-limited (Heartbeat,
// TxOffer), plus BlockRequest — a real, independent audit finding (Phase
// 2, low/medium): unlike every other unthrottled type here, a
// BlockRequest triggers real, synchronous per-request disk I/O
// (handleBlockRequest reads up to MaxCatchUpBlocks blocks from the
// store), so a peer dialing a fresh stream as fast as it can and sending
// BlockRequest repeatedly was a cheap, real I/O-amplification DoS this
// limiter otherwise did nothing to slow down. Every other message type
// always passes through unthrottled (though a real deployment may choose
// to extend the set further via governance).
func (r *RateLimiter) Allow(p peer.ID, t MessageType, now time.Time) bool {
	if t != MsgHeartbeat && t != MsgTxOffer && t != MsgBlockRequest {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if until, dropped := r.droppedUntil[p]; dropped {
		if now.Before(until) {
			return false
		}
		delete(r.droppedUntil, p)
	}

	peerBuckets, ok := r.buckets[p]
	if !ok {
		peerBuckets = map[MessageType]*bucket{}
		r.buckets[p] = peerBuckets
	}
	b, ok := peerBuckets[t]
	if !ok {
		b = newBucket(r.cfg.Capacity, r.cfg.RefillRate, now)
		peerBuckets[t] = b
	}
	if !b.take(now) {
		r.droppedUntil[p] = now.Add(r.cfg.Cooldown)
		return false
	}
	return true
}

// IsDropped reports whether p is currently in its post-flood cooldown.
func (r *RateLimiter) IsDropped(p peer.ID, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.droppedUntil[p]
	return ok && now.Before(until)
}
