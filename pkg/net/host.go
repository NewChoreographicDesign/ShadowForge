// Package net is the ShadowForge P2P layer: a libp2p host secured with
// Noise, a small message protocol for the types spec section 6 names
// (Heartbeat, TxOffer, StageVote, BlockAnnounce, MegabatchPart,
// ContainerSync, SilentPad), and a per-peer rate limiter.
//
// Scope: spec 6 also calls for "bootstrap node list in genesis plus DHT."
// This package implements bootstrap-list connection (Connect) — the
// concrete, testable mechanism a genesis file actually needs — and leaves
// full Kademlia DHT peer discovery as a documented extension point
// (go-libp2p's kad-dht package plugs into the same host.Host this package
// builds); a from-scratch DHT is a distinct, large subsystem that adds
// bootstrap capability the genesis list already covers for the
// four-node Docker network in spec 6 / 18.1's Phase 2 checkpoint.
package net

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	ma "github.com/multiformats/go-multiaddr"
)

// ProtocolID is the ShadowForge consensus-message stream protocol.
const ProtocolID = "/shadowforge/1.0.0"

// NewHost builds a libp2p host secured with Noise XX (spec 6: "Transport:
// libp2p with Noise XX (or equivalent Noise handshake). All consensus
// messages encrypted."). listenAddr is a multiaddr string, e.g.
// "/ip4/127.0.0.1/tcp/0" (port 0 = pick a free port).
func NewHost(listenAddr string) (host.Host, error) {
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Security(noise.ID, noise.New),
	)
	if err != nil {
		return nil, fmt.Errorf("net: create libp2p host: %w", err)
	}
	return h, nil
}

// Connect dials a bootstrap peer by its multiaddr string (which must
// include a /p2p/<peerID> suffix) and adds it to the host's peerstore,
// implementing the "bootstrap node list in genesis" half of spec 6's
// discovery mechanism.
func Connect(ctx context.Context, h host.Host, addr string) error {
	maddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("net: parse bootstrap addr %q: %w", addr, err)
	}
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("net: parse peer info from %q: %w", addr, err)
	}
	h.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
	if err := h.Connect(ctx, *info); err != nil {
		return fmt.Errorf("net: connect to bootstrap peer %s: %w", info.ID, err)
	}
	return nil
}

// FullAddr returns h's dialable multiaddr(s) including its peer ID, for
// use as another node's bootstrap address.
func FullAddr(h host.Host) []string {
	var out []string
	pid := h.ID()
	for _, a := range h.Addrs() {
		out = append(out, a.String()+"/p2p/"+pid.String())
	}
	return out
}
