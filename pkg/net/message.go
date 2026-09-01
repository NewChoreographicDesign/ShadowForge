package net

import (
	"encoding/json"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// MessageType enumerates spec 6's message types ("Heartbeat, TxOffer,
// StageVote, BlockAnnounce, MegabatchPart, ContainerSync, SilentPad") plus
// BlockProposal, a necessary addition spec 6 doesn't spell out at the wire
// level: real cross-node BFT voting (spec 5.7) requires committee members
// to vote on one agreed-upon batch, not their own independently-chosen
// mempool contents, so someone has to broadcast the batch before anyone
// can vote on it.
type MessageType string

const (
	MsgHeartbeat     MessageType = "Heartbeat"
	MsgTxOffer       MessageType = "TxOffer"
	MsgBlockProposal MessageType = "BlockProposal"
	MsgStageVote     MessageType = "StageVote"
	MsgBlockAnnounce MessageType = "BlockAnnounce"
	MsgMegabatchPart MessageType = "MegabatchPart"
	MsgContainerSync MessageType = "ContainerSync"
	MsgSilentPad     MessageType = "SilentPad"
	// MsgBlockRequest/MsgBlockResponse are a second necessary addition
	// spec 6's message list doesn't spell out, for the identical reason
	// MsgBlockProposal is: real multi-block catch-up (a node that falls
	// more than one block behind its peers) needs a way to ask a
	// specific peer for the blocks it's missing and receive them back,
	// which a purely broadcast/push protocol (every other message type
	// here) cannot do on its own. See pkg/validator's own doc for why
	// this was previously out of scope and what closing it now covers.
	MsgBlockRequest  MessageType = "BlockRequest"
	MsgBlockResponse MessageType = "BlockResponse"
)

// Envelope is the wire format for every message this protocol sends: a
// type tag plus an opaque JSON payload whose shape depends on Type.
type Envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewEnvelope marshals payload into an Envelope of the given type.
func NewEnvelope(t MessageType, payload interface{}) (Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: t, Payload: b}, nil
}

// HeartbeatPayload is sent every HeartbeatInterval (spec 5.4.2). PubKey is
// this validator's real Dilithium public key, so peers can build the
// identity registry real StageVote/Block verification needs
// (pkg/chain.PubKeyLookup) — a trust-on-first-heartbeat substitute for a
// real on-chain NFT-mint identity binding, which this reference build
// doesn't wire end to end (see README's Scope section).
type HeartbeatPayload struct {
	NFT       types.NFTID `json:"nft"`
	PubKey    []byte      `json:"pub_key"`
	Timestamp int64       `json:"timestamp"`
	// IsSentinel marks this heartbeat as coming from a protocol-run
	// sentinel validator rather than a civilian one (spec 5.5). Peers use
	// it to count "online civilians" separately from sentinels — the
	// input consensus.SentinelManager.Evaluate needs to decide whether
	// sentinels should activate or withdraw.
	IsSentinel bool `json:"is_sentinel,omitempty"`
}

// TxOfferPayload carries a shielded transaction into the mempool.
type TxOfferPayload struct {
	Tx types.ShieldedTx `json:"tx"`
}

// BlockProposalPayload is broadcast by the height's deterministically
// assigned proposer (consensus.AssignCommittee[0]) before any committee
// member runs the pipeline. Committee members vote on this exact batch,
// never on their own independently-drained mempool — that's what lets
// every honest node that executes it compute the identical candidate root.
type BlockProposalPayload struct {
	Height    uint64             `json:"height"`
	Epoch     uint64             `json:"epoch"`
	Proposer  types.NFTID        `json:"proposer"`
	Batch     []types.ShieldedTx `json:"batch"`
	Timestamp int64              `json:"timestamp"`
	// DualTrack marks this proposal as an outage-recovery batch combining
	// live traffic (Track A) with drained backlog (Track B) — spec 5.6.
	// Carried through unchanged into the resulting types.Block.DualTrack.
	DualTrack bool `json:"dual_track,omitempty"`
}

// StageVotePayload is one committee member's BFT vote for a candidate
// block header hash (spec 5.7; types.HashBlock is what CandidateHash
// signs over).
type StageVotePayload struct {
	Height        uint64             `json:"height"`
	Validator     types.NFTID        `json:"validator"`
	CandidateHash types.Hash         `json:"candidate_hash"`
	Sig           types.DilithiumSig `json:"sig"`
}

// BlockAnnouncePayload carries a fully-assembled, quorum-voted block so
// any node — committee member or not — can independently verify (real
// signature checks against the announced Votes, real chain.Append
// height/PrevHash validation) and adopt it.
type BlockAnnouncePayload struct {
	Block types.Block `json:"block"`
}

// MaxMegabatchPartBytes bounds one MegabatchPartPayload chunk's Data
// field — real defense-in-depth against an oversized single chunk,
// mirroring MaxCatchUpBlocks's identical role for block catch-up: a
// recovery megabatch too large for one chunk is sent as more chunks
// (PartCount > 1), not one unbounded one.
const MaxMegabatchPartBytes = 64 * 1024 // 64 KiB per part

// MaxMegabatchParts bounds how many parts one reassembly will ever
// buffer, regardless of what a peer's own PartCount claims — real
// defense-in-depth against a peer claiming an unbounded part count purely
// to make a receiver allocate an unbounded []byte slice before a single
// real byte has arrived. A real megabatch (consensus.MegabatchMultiplier
// * a node's own MaxBatchSize, real Dilithium-signed transactions) tops
// out at a few hundred parts at MaxMegabatchPartBytes — this leaves
// generous headroom above that while still bounding the worst case.
const MaxMegabatchParts = 1024

// MegabatchPartPayload carries one chunk of an outage-recovery megabatch
// (spec 5.6) — a real, disclosed side channel broadcast alongside (not
// instead of) the actual dual-track BlockProposal/BlockAnnounce path:
// the committee still only ever votes on and commits whatever fit inside
// MaxBatchBytes's own budget (see pkg/validator's buildProposalBatch),
// exactly as before. What this closes is the gap where a proposer's
// *entire* recovery megabatch — everything OutageController.BuildMegabatch
// drained this round, before that same byte-budget trim — was never
// visible to the wider network at all, only to whoever ended up on the
// committee. Every real node, not just committee members, can now
// reassemble the full megabatch a recovery round claims to be draining
// and cross-check it against what actually lands in the committed chain
// afterward — real transparency into outage recovery, not a consensus
// input: a peer that reassembles a bogus or incomplete announcement can
// only ever notice and log a discrepancy, never have it affect its own
// chain state, exactly like every other observability signal this
// codebase already treats this message type's row as (see MessageType's
// own doc).
//
// Height correlates every part of one recovery round's announcement
// (there is no other framing to disambiguate concurrent broadcasts from
// the same or different senders); PartIndex/PartCount let the receiver
// know when every chunk of that one announcement has arrived.
type MegabatchPartPayload struct {
	Height    uint64 `json:"height"`
	PartIndex int    `json:"part_index"`
	PartCount int    `json:"part_count"`
	Data      []byte `json:"data"`
}

// ContainerSyncPayload carries an enterprise container's aggregated
// mega-batch sync (spec 15.3).
type ContainerSyncPayload struct {
	ContainerID string     `json:"container_id"`
	RootHash    types.Hash `json:"root_hash"`
}

// SilentPadPayload is a null ZK pad used to keep circuits warm and absorb
// burst load (spec 15.4).
type SilentPadPayload struct {
	Nonce []byte `json:"nonce"`
}

// MaxCatchUpBlocks bounds how many blocks a single BlockRequest may ask
// for (and a single BlockResponse may return) — real defense-in-depth
// against a request for an unbounded range turning into an unbounded
// response, mirroring MaxEnvelopeSize/MaxBatchBytes's identical role
// elsewhere in this codebase. A node needing to catch up by more than
// this many blocks simply issues another request once it has processed
// the first response's blocks — real, incremental catch-up, not a
// single unbounded transfer.
const MaxCatchUpBlocks = 200

// BlockRequestPayload asks the receiving peer for every block it has
// stored with FromHeight <= height <= ToHeight (inclusive), sent to one
// specific peer (net.Node.Send), not broadcast — see MsgBlockRequest's
// own doc for why this, unlike every other message in this protocol,
// needs a request/response shape rather than push/broadcast.
type BlockRequestPayload struct {
	FromHeight uint64 `json:"from_height"`
	ToHeight   uint64 `json:"to_height"`
}

// BlockResponsePayload answers a BlockRequestPayload with every real,
// already-committed block the responder actually has in the requested
// range, in ascending height order, capped at MaxCatchUpBlocks — it may
// be shorter than requested (the responder's own head is behind
// ToHeight, or it simply doesn't have every block in range), never
// fabricated to fill a gap: the requester independently re-verifies
// every block it receives (the same real replay-and-check path a
// BlockAnnounce gets) before ever adopting it, exactly like this
// protocol already does for a single announced block.
type BlockResponsePayload struct {
	Blocks []types.Block `json:"blocks"`
}
