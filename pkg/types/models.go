package types

import "github.com/shadowforge/shadowforge-l1/pkg/decimal"

// ComputeTxID implements spec 4.1 exactly: "TxID: Hash of the shielded
// transaction blob (proof + commitments + nullifier)." Both the sender
// (when building a transaction) and a validator (when checking one, per
// spec 5.3 Stage 2's "well-formedness" check) must compute this the same
// way, so a submitted TxID that doesn't match its own proof/commitments/
// nullifier is detectably tampered with.
func ComputeTxID(proof []byte, commitments []Hash, nullifier Hash) Hash {
	parts := make([][]byte, 0, len(commitments)+2)
	parts = append(parts, proof)
	for _, c := range commitments {
		parts = append(parts, c[:])
	}
	parts = append(parts, nullifier[:])
	return SumHash(parts...)
}

// TxKind enumerates the kinds a ShieldedTx can be (spec 4.2).
type TxKind uint8

const (
	TxTransfer TxKind = iota
	TxMint
	TxVote
	TxBankDeposit
	TxBankWithdraw
	TxNFTTrait
	TxContainerSync
)

func (k TxKind) String() string {
	switch k {
	case TxTransfer:
		return "Transfer"
	case TxMint:
		return "Mint"
	case TxVote:
		return "Vote"
	case TxBankDeposit:
		return "BankDeposit"
	case TxBankWithdraw:
		return "BankWithdraw"
	case TxNFTTrait:
		return "NFTTrait"
	case TxContainerSync:
		return "ContainerSync"
	default:
		return "Unknown"
	}
}

// StageSet is a bitset over the 5 pipeline stages, tracking StageHints
// (spec 4.2: "which of the 5 stages already passed, for pipeline retry").
type StageSet uint8

func (s StageSet) Has(stage int) bool { return stage >= 1 && stage <= 5 && s&(1<<uint(stage-1)) != 0 }
func (s StageSet) With(stage int) StageSet {
	if stage < 1 || stage > 5 {
		return s
	}
	return s | (1 << uint(stage-1))
}
func (s StageSet) Complete() bool { return s == 0b11111 }

// DilithiumSig is a detached post-quantum signature (spec 8.5).
type DilithiumSig []byte

// ShieldedTx is the persisted, never-cleartext transaction object (spec 4.2).
type ShieldedTx struct {
	TxID         Hash
	Nullifier    Hash   // prevents double-spend of the note
	Commitments  []Hash // new notes created
	Proof        []byte // gnark proof bytes
	FeeCommit    Hash   // fee paid to Vault, also shielded
	Memo         []byte // optional encrypted memo for receiver
	Sig          DilithiumSig
	SignerPubKey []byte // Dilithium public key the wallet signed Sig with (spec 8.5)
	Kind         TxKind
	StageHints   StageSet
	ContainerID  *ID // nil unless originated in an enterprise container

	// Public inputs bound per Kind (spec 4.2: "TxKind determines which
	// extra public inputs the circuit must bind"). Only the fields
	// relevant to Kind are populated; the rest stay zero.
	BankPublicInputs  *BankPublicInputs
	VotePublicInputs  *VotePublicInputs
	TraitPublicInputs *TraitPublicInputs

	// TransferPublicInputs carries the exact, fixed-shape public inputs
	// pkg/zk's TransferCircuit was actually proved and must be verified
	// against, for Kind Transfer. The top-level Nullifier/Commitments
	// fields above remain the spec-4.2 canonical shape (Nullifier is this
	// transfer's primary spent-note nullifier; Commitments lists the
	// output notes); TransferPublicInputs additionally carries any padding
	// notes the circuit's fixed NumInputs/NumOutputs shape required (spec
	// 4.2 leaves the exact multi-input encoding open — "TxKind determines
	// which extra public inputs the circuit must bind" — this is that
	// binding for Transfer).
	TransferPublicInputs *TransferPublicInputs
}

// TransferPublicInputs mirrors pkg/zk.TransferCircuit's public inputs
// exactly (MerkleRoot, all Nullifiers, all output Commitments, Fee), so
// verification never has to guess field ordering.
type TransferPublicInputs struct {
	MerkleRoot Hash
	Nullifiers []Hash
	OutCommits []Hash
	FeeAmount  uint64
}

// BankPublicInputs binds oracle price, ATR, and buffer for BankDeposit /
// BankWithdraw kinds.
type BankPublicInputs struct {
	OraclePriceUSD decimal.Decimal
	ATRUSD         decimal.Decimal
	BufferUSD      decimal.Decimal
}

// VotePublicInputs binds a proposal id and a yes/no commitment for Vote kind.
type VotePublicInputs struct {
	ProposalID ID
	Commitment Hash
}

// TraitPublicInputs binds the trait key and a delta commitment for NFTTrait
// kind.
type TraitPublicInputs struct {
	Key             string
	DeltaCommitment Hash
}

// Vote is a single BFT signature from an assigned stage validator (spec 4.3).
type Vote struct {
	Validator NFTID
	StateRoot Hash
	Sig       DilithiumSig
}

// Block is one committed batch (spec 4.3).
type Block struct {
	Height      BlockHeight
	Epoch       EpochNumber
	PrevHash    Hash
	Timestamp   int64 // unix ms, proposer local, bounded by network skew rule
	Batch       []ShieldedTx
	TxRoot      Hash
	StateRoot   Hash
	DARoot      Hash
	Proposer    NFTID
	ProposerSig DilithiumSig
	Votes       []Vote
	DualTrack   bool // true if this block is the backlog track of a megabatch
}

// Note is a private unspent value object; only Commitment is public
// (spec 4.4).
type Note struct {
	Commitment Hash    // public
	Value      uint64  // private, inside the circuit
	OwnerPk    []byte  // private
	Rho        []byte  // nullifier seed, private
	Asset      AssetID // SFG at launch; other assets only inside Bank custody
}

// AccountMeta is the per-address metadata record (spec 4.4).
type AccountMeta struct {
	NFT           *NFTID
	TrustPoints   uint64
	ValidateOn    bool
	MintOn        bool
	RevealKeyHash *Hash  // exists if user registered a selective-disclosure capability
	DailyBankUsed uint64 // USD-equivalent, reset at UTC midnight
}

// ValidatorNFT is the soulbound validator credential (spec 4.5).
type ValidatorNFT struct {
	ID       NFTID
	Owner    Address // soulbound; transfer disabled until governance unlocks a trait
	MintedAt BlockHeight
	Traits   map[string]string // e.g. badge=RecoveryHero, dept=Finance
	TP       uint64
	Slashed  bool
}

// QueueItem is one entry in the revolver deque (spec 4.6).
type QueueItem struct {
	NFT           NFTID
	Address       Address
	JoinedAt      int64
	LastBeat      int64
	CooldownUntil int64 // set to now+1h when a node goes offline
	TP            uint64
}

// HoldStatus is the lifecycle state of a BankHold (spec 4.7).
type HoldStatus uint8

const (
	HoldOpen HoldStatus = iota
	HoldLocked24h
	HoldClosing
	HoldClosed
)

func (s HoldStatus) String() string {
	switch s {
	case HoldOpen:
		return "Open"
	case HoldLocked24h:
		return "Locked24h"
	case HoldClosing:
		return "Closing"
	case HoldClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// ATRPoint is one daily ATR snapshot recorded against an open BankHold.
type ATRPoint struct {
	Timestamp int64
	ATRUSD    decimal.Decimal
}

// BankHold is the open record of an external-asset deposit (spec 4.7).
type BankHold struct {
	HoldID         Hash
	Owner          Address
	ExternalAsset  AssetID
	ExternalAmount decimal.Decimal
	EntryPriceUSD  decimal.Decimal // oracle snapshot
	EntryATR       decimal.Decimal // current ATR at deposit, USD
	EntryBuffer    decimal.Decimal // 2.5 * EntryATR
	EntryFee       decimal.Decimal // 0.1% of (gross - buffer)
	SFGIssued      uint64
	OpenedAt       int64
	DailySnapshots []ATRPoint
	Status         HoldStatus
	CycleCount30d  uint // for >3/month surcharge
}
