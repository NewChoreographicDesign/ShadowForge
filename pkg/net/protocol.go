package net

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// MaxEnvelopeSize caps the total bytes handleStream will read from one
// stream. Send opens a fresh stream per Envelope, so in practice this caps
// one message's wire size; without it, a peer streaming an unbounded JSON
// payload could exhaust this node's memory. go-libp2p's default resource
// manager provides a coarser, connection-level backstop, but this is the
// direct defense (the rate limiter is the first DoS brake spec 6 calls
// for, against many small messages; this is the second, against one
// oversized one).
const MaxEnvelopeSize = 4 << 20 // 4 MiB

// StreamIdleTimeout bounds how long handleStream will wait for the next
// message on an open stream before giving up. Without this, a peer that
// opens a stream and then sends nothing (or trickles bytes) ties up a
// goroutine indefinitely — a slow-loris-style resource exhaustion attack.
const StreamIdleTimeout = 30 * time.Second

// Handler processes one received Envelope from peer p.
type Handler func(p peer.ID, env Envelope)

// Node wraps a libp2p host with the ShadowForge message protocol: envelope
// framing, dispatch to a Handler, and per-peer rate limiting (spec 6).
type Node struct {
	Host    host.Host
	Limiter *RateLimiter

	handler Handler
}

// NewNode registers the ShadowForge stream protocol on h. handler is
// invoked for every Envelope that passes the rate limiter.
func NewNode(h host.Host, limiter *RateLimiter, handler Handler) *Node {
	if limiter == nil {
		limiter = NewRateLimiter(DefaultRateLimiterConfig())
	}
	n := &Node{Host: h, Limiter: limiter, handler: handler}
	h.SetStreamHandler(protocol.ID(ProtocolID), n.handleStream)
	return n
}

func (n *Node) handleStream(s network.Stream) {
	defer func() { _ = s.Close() }()
	remote := s.Conn().RemotePeer()
	dec := json.NewDecoder(io.LimitReader(s, MaxEnvelopeSize))
	for {
		if err := s.SetReadDeadline(time.Now().Add(StreamIdleTimeout)); err != nil {
			return // stream doesn't support deadlines or is already closed
		}
		var env Envelope
		if err := dec.Decode(&env); err != nil {
			return // EOF, malformed stream, oversized message, or idle timeout
		}
		if !n.Limiter.Allow(remote, env.Type, time.Now()) {
			return // rate-limited: drop the rest of this stream too
		}
		if n.handler != nil {
			n.handler(remote, env)
		}
	}
}

// Send opens a fresh stream to p and writes one Envelope.
func (n *Node) Send(ctx context.Context, p peer.ID, env Envelope) error {
	s, err := n.Host.NewStream(ctx, p, protocol.ID(ProtocolID))
	if err != nil {
		return fmt.Errorf("net: open stream to %s: %w", p, err)
	}
	defer func() { _ = s.Close() }()
	if err := json.NewEncoder(s).Encode(env); err != nil {
		return fmt.Errorf("net: write envelope to %s: %w", p, err)
	}
	return nil
}

// Broadcast sends env to every currently connected peer, collecting (not
// stopping on) individual failures.
func (n *Node) Broadcast(ctx context.Context, env Envelope) map[peer.ID]error {
	errs := map[peer.ID]error{}
	for _, p := range n.Host.Network().Peers() {
		if err := n.Send(ctx, p, env); err != nil {
			errs[p] = err
		}
	}
	return errs
}
