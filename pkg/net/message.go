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
// doesn't wire end to end (see docs/ARCHITECTURE.md's scope notes).
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

// MegabatchPartPayload carries one chunk of an outage-recovery megabatch
// (spec 5.6).
type MegabatchPartPayload struct {
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
