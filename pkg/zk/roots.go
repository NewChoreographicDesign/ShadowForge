package zk

import "sync"

// RootHistory is the canonical, network-agreed history of every
// commitment-tree root a real Transfer proof may anchor to.
//
// This closes a real gap live testing surfaced: Stage 1's proof
// verification alone only proves a proof is internally consistent for
// whatever root the prover claims — it never proved that claimed root
// was one the network actually produced. Nothing had ever exercised Kind
// Transfer's full path live before (every earlier live network test in
// this codebase's history ran with -skip-zk-setup, which rejects
// Transfer outright before reaching this code), so the gap went
// undetected until real client-side transfer support required tracing
// the path end to end. Without this check, any internally-consistent
// proof of membership in a tree the prover invented themselves — with
// notes nobody else has ever seen — would pass. Every honest node now
// builds an identical RootHistory by inserting into the same Tree, in
// the same committed order (pkg/tx's pipeline, Stage 4), so Contains is
// what actually anchors a Transfer proof to reality rather than merely
// to internal consistency.
//
// Recording every historical root rather than only the latest matters
// for real usability: a wallet builds its proof against whatever root it
// last observed, and by the time that transaction commits, other
// transactions ordered earlier in the same or a prior batch may have
// already advanced the canonical tree past it. Rejecting anything but
// the single newest root would make ordinary concurrent use fail
// unpredictably.
//
// A real, disclosed scaling limit: at MerkleDepth's now-production
// capacity (see that constant's own doc), nothing bounds how many roots
// can accumulate here except real transaction volume over the chain's
// entire lifetime — unlike the old 16-leaf placeholder depth, where the
// tree's own capacity capped this at a handful of entries for free.
// Contains is an O(1) map lookup (not a linear scan) precisely because
// of that: real transaction volume, not MerkleDepth, is what could
// eventually make this history's memory footprint worth revisiting (a
// pruning or windowing policy for roots older than any proof still in
// flight would reclaim it) — a separate, later concern this build does
// not need to solve to make MerkleDepth itself real.
type RootHistory struct {
	mu    sync.Mutex
	roots map[FieldElement]struct{}
}

// NewRootHistory seeds a RootHistory with initialRoot — the empty tree's
// own root, so a fresh node and a freshly-initialized RootHistory always
// agree on the one root that's valid before any real note has ever been
// committed.
func NewRootHistory(initialRoot FieldElement) *RootHistory {
	return &RootHistory{roots: map[FieldElement]struct{}{initialRoot: {}}}
}

// Record appends a newly-canonical root — called once per real Transfer
// commit (pkg/tx's pipeline, Stage 4), immediately after inserting that
// transfer's output commitments into the same Tree this history tracks.
func (h *RootHistory) Record(root FieldElement) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roots[root] = struct{}{}
}

// Contains reports whether root is one this history has ever recorded —
// what Stage 1 checks a Transfer proof's claimed anchor against.
func (h *RootHistory) Contains(root FieldElement) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.roots[root]
	return ok
}
