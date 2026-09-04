package net_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func newTestHost(t *testing.T) *shadownet.Node {
	t.Helper()
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return shadownet.NewNode(h, nil, nil)
}

// TestTwoNodesConnectAndExchangeHeartbeat proves real, Noise-encrypted
// libp2p connectivity end to end: dial by bootstrap multiaddr, open a
// ShadowForge protocol stream, and deliver a decoded Heartbeat to the
// receiver's handler.
func TestTwoNodesConnectAndExchangeHeartbeat(t *testing.T) {
	hostA, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host A: %v", err)
	}
	defer func() { _ = hostA.Close() }()
	hostB, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host B: %v", err)
	}
	defer func() { _ = hostB.Close() }()

	received := make(chan shadownet.HeartbeatPayload, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	nodeB := shadownet.NewNode(hostB, nil, func(p peer.ID, env shadownet.Envelope) {
		defer wg.Done()
		if env.Type != shadownet.MsgHeartbeat {
			t.Errorf("unexpected message type %s", env.Type)
			return
		}
		var hb shadownet.HeartbeatPayload
		if err := unmarshalPayload(env, &hb); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		received <- hb
	})
	_ = nodeB

	nodeA := shadownet.NewNode(hostA, nil, nil)

	addrs := shadownet.FullAddr(hostB)
	if len(addrs) == 0 {
		t.Fatalf("host B has no listen addresses")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := shadownet.Connect(ctx, hostA, addrs[0]); err != nil {
		t.Fatalf("connect: %v", err)
	}

	wantNFT := types.NFTID{1, 2, 3}
	env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
		NFT: wantNFT, Timestamp: 42,
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	if err := nodeA.Send(ctx, hostB.ID(), env); err != nil {
		t.Fatalf("send: %v", err)
	}

	waitOrTimeout(t, &wg, 5*time.Second)
	select {
	case hb := <-received:
		if hb.NFT != wantNFT || hb.Timestamp != 42 {
			t.Fatalf("unexpected heartbeat payload: %+v", hb)
		}
	default:
		t.Fatalf("handler did not deliver a heartbeat")
	}
}

func waitOrTimeout(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for handler")
	}
}

func unmarshalPayload(env shadownet.Envelope, v interface{}) error {
	return json.Unmarshal(env.Payload, v)
}

func TestNewHostHasAnID(t *testing.T) {
	n := newTestHost(t)
	if n.Host.ID() == "" {
		t.Fatalf("expected a non-empty peer ID")
	}
}

func TestRateLimiterDropsFloodingPeer(t *testing.T) {
	rl := shadownet.NewRateLimiter(shadownet.RateLimiterConfig{
		Capacity: 3, RefillRate: 1, Cooldown: time.Minute,
	})
	p := peer.ID("test-peer")
	now := time.Now()

	for i := 0; i < 3; i++ {
		if !rl.Allow(p, shadownet.MsgHeartbeat, now) {
			t.Fatalf("expected message %d to be allowed within capacity", i)
		}
	}
	if rl.Allow(p, shadownet.MsgHeartbeat, now) {
		t.Fatalf("expected the 4th message to exceed capacity and be dropped")
	}
	if !rl.IsDropped(p, now) {
		t.Fatalf("expected peer to be in cooldown after exceeding the bucket")
	}
	// Still dropped mid-cooldown, even for a different rate-limited type.
	if rl.Allow(p, shadownet.MsgTxOffer, now.Add(time.Second)) {
		t.Fatalf("expected peer to remain dropped during cooldown")
	}
	// Cooldown expires.
	if !rl.Allow(p, shadownet.MsgHeartbeat, now.Add(time.Minute+time.Second)) {
		t.Fatalf("expected peer to be allowed again after cooldown expires")
	}
}

func TestRateLimiterIgnoresUnthrottledTypes(t *testing.T) {
	rl := shadownet.NewRateLimiter(shadownet.RateLimiterConfig{Capacity: 1, RefillRate: 0, Cooldown: time.Minute})
	p := peer.ID("test-peer")
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !rl.Allow(p, shadownet.MsgBlockAnnounce, now) {
			t.Fatalf("BlockAnnounce is not spec-6 rate-limited and should never be dropped")
		}
	}
}

// TestRateLimiterThrottlesBlockRequest is a real, independent audit
// finding (Phase 2, low/medium): unlike every other message type this
// limiter leaves unthrottled, BlockRequest triggers real per-request disk
// I/O (handleBlockRequest reads up to MaxCatchUpBlocks real blocks from
// the store), so a peer flooding it was a cheap I/O-amplification DoS.
// This proves it's now bounded by the same token bucket as
// Heartbeat/TxOffer.
func TestRateLimiterThrottlesBlockRequest(t *testing.T) {
	rl := shadownet.NewRateLimiter(shadownet.RateLimiterConfig{Capacity: 1, RefillRate: 0, Cooldown: time.Minute})
	p := peer.ID("test-peer")
	now := time.Now()
	if !rl.Allow(p, shadownet.MsgBlockRequest, now) {
		t.Fatalf("expected the first BlockRequest to be allowed (capacity 1)")
	}
	if rl.Allow(p, shadownet.MsgBlockRequest, now) {
		t.Fatalf("expected a second immediate BlockRequest to be throttled (capacity exhausted, no refill)")
	}
}

// TestOversizedMessageRejected proves the MaxEnvelopeSize guard actually
// stops an oversized payload rather than buffering it into memory: a raw
// stream (bypassing Send/Envelope framing) writes well past the cap, and
// the receiving handler must never fire.
func TestOversizedMessageRejected(t *testing.T) {
	hostA, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host A: %v", err)
	}
	defer func() { _ = hostA.Close() }()
	hostB, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("host B: %v", err)
	}
	defer func() { _ = hostB.Close() }()

	handlerFired := make(chan struct{}, 1)
	shadownet.NewNode(hostB, nil, func(p peer.ID, env shadownet.Envelope) {
		handlerFired <- struct{}{}
	})
	shadownet.NewNode(hostA, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addrs := shadownet.FullAddr(hostB)
	if err := shadownet.Connect(ctx, hostA, addrs[0]); err != nil {
		t.Fatalf("connect: %v", err)
	}

	s, err := hostA.NewStream(ctx, hostB.ID(), shadownet.ProtocolID)
	if err != nil {
		t.Fatalf("open raw stream: %v", err)
	}
	// A well-formed-looking JSON envelope whose payload alone exceeds
	// MaxEnvelopeSize, sent as one big write.
	oversized := append([]byte(`{"type":"Heartbeat","payload":"`), make([]byte, shadownet.MaxEnvelopeSize+1024)...)
	oversized = append(oversized, []byte(`"}`)...)
	for i := range oversized[len(`{"type":"Heartbeat","payload":"`):] {
		// fill with a harmless ASCII byte so it's still syntactically
		// plausible up to the point the reader gives up
		idx := len(`{"type":"Heartbeat","payload":"`) + i
		if idx >= len(oversized)-2 {
			break
		}
		oversized[idx] = 'a'
	}
	_, writeErr := s.Write(oversized)
	_ = s.CloseWrite()
	_ = writeErr // a write error here (e.g. reset by peer) is an acceptable outcome too

	select {
	case <-handlerFired:
		t.Fatalf("handler must not fire for a message exceeding MaxEnvelopeSize")
	case <-time.After(2 * time.Second):
		// expected: nothing delivered
	}
}

func TestConnectFailsForUnreachableAddr(t *testing.T) {
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer func() { _ = h.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	unreachable := "/ip4/127.0.0.1/tcp/1/p2p/QmVzKJj1Uv2pJTL5RJXrKvGVSPUZJKquBmk4bYS4gh1rvS"
	if err := shadownet.Connect(ctx, h, unreachable); err == nil {
		t.Fatalf("expected connecting to an unreachable peer to fail")
	}
}
