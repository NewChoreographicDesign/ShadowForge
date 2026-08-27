package net

import "encoding/json"

// MessageType enumerates spec 6's message types: "Heartbeat, TxOffer,
// StageVote, BlockAnnounce, MegabatchPart, ContainerSync, SilentPad."
type MessageType string

const (
	MsgHeartbeat     MessageType = "Heartbeat"
	MsgTxOffer       MessageType = "TxOffer"
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

// HeartbeatPayload is sent every HeartbeatInterval (spec 5.4.2).
type HeartbeatPayload struct {
	NFT       string `json:"nft"`
	Timestamp int64  `json:"timestamp"`
}

// TxOfferPayload carries a shielded transaction blob into the mempool.
type TxOfferPayload struct {
	TxBytes []byte `json:"tx_bytes"`
}

// StageVotePayload is one validator's BFT vote for a candidate state root
// (spec 5.7).
type StageVotePayload struct {
	Validator string `json:"validator"`
	StateRoot string `json:"state_root"`
	Sig       []byte `json:"sig"`
}

// BlockAnnouncePayload announces a newly committed block.
type BlockAnnouncePayload struct {
	Height    uint64 `json:"height"`
	BlockHash string `json:"block_hash"`
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
	ContainerID string `json:"container_id"`
	RootHash    string `json:"root_hash"`
}

// SilentPadPayload is a null ZK pad used to keep circuits warm and absorb
// burst load (spec 15.4).
type SilentPadPayload struct {
	Nonce []byte `json:"nonce"`
}
