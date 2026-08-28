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
// unpredictably. MerkleDepth's own tiny (TreeSize-leaf) capacity already
// bounds how many roots can ever exist across this build's lifetime, so
// keeping the full history costs nothing meaningful.
type RootHistory struct {
	mu    sync.Mutex
	roots []FieldElement
}

// NewRootHistory seeds a RootHistory with initialRoot — the empty tree's
// own root, so a fresh node and a freshly-initialized RootHistory always
// agree on the one root that's valid before any real note has ever been
// committed.
func NewRootHistory(initialRoot FieldElement) *RootHistory {
	return &RootHistory{roots: []FieldElement{initialRoot}}
}

// Record appends a newly-canonical root — called once per real Transfer
// commit (pkg/tx's pipeline, Stage 4), immediately after inserting that
// transfer's output commitments into the same Tree this history tracks.
func (h *RootHistory) Record(root FieldElement) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roots = append(h.roots, root)
}

// Contains reports whether root is one this history has ever recorded —
// what Stage 1 checks a Transfer proof's claimed anchor against.
func (h *RootHistory) Contains(root FieldElement) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.roots {
		if r.Equal(&root) {
			return true
		}
	}
	return false
}
