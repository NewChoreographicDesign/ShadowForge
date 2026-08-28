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
	// TxVoteReveal opens a sealed TxVote ballot at epoch end: Approve and
	// Nonce, checked against the Commitment the earlier TxVote bound (see
	// ComputeVoteCommitment) before the choice counts toward a real tally
	// (spec 17.4's "Votes accumulate as ZKP ballots during the epoch" —
	// the spec names the commit step but not a reveal mechanism, so this
	// commit-reveal scheme is this build's own implementation decision,
	// documented here rather than left unspecified).
	TxVoteReveal
	// TxNFTMint claims the free, one-per-wallet soulbound validator NFT
	// spec 10.1 describes ("User requests a micro-drop of SFG... Mint UI
	// presents CAPTCHA and a proof-of-humanity challenge... Contract
	// enforces one NFT per wallet"). It is the real, live origination
	// path this build was missing for ValidatorNFT records — see
	// NFTMintPublicInputs and pkg/nft.Mint, which pkg/tx's pipeline calls
	// to enforce a real, signed proof-of-humanity attestation rather than
	// a caller-supplied bool. Holding a real NFT minted this way is also
	// what makes TxVote/TxVoteReveal's voter eligibility real instead of
	// an unchecked, freely-forgeable identity.
	TxNFTMint
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
	case TxVoteReveal:
		return "VoteReveal"
	case TxNFTMint:
		return "NFTMint"
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
	BankPublicInputs       *BankPublicInputs
	VotePublicInputs       *VotePublicInputs
	VoteRevealPublicInputs *VoteRevealPublicInputs
	TraitPublicInputs      *TraitPublicInputs
	NFTMintPublicInputs    *NFTMintPublicInputs

	// VoteEligibility is the real anonymous ZK proof of "one NFT, one
	// vote" eligibility required alongside VotePublicInputs/
	// VoteRevealPublicInputs — see VoteEligibilityProof's own doc.
	VoteEligibility *VoteEligibilityProof

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
// BankWithdraw kinds. Asset identifies which external asset (e.g. BTC, ETH)
// OraclePriceUSD/ATRUSD claim to price — the pipeline's Stage 4 needs it to
// know which real oracle.Quorum reading to cross-check the claim against;
// the zero value is only valid when no oracle is wired (Deps.Oracle == nil),
// which existing single-asset tests rely on.
type BankPublicInputs struct {
	Asset          AssetID
	OraclePriceUSD decimal.Decimal
	ATRUSD         decimal.Decimal
	BufferUSD      decimal.Decimal
}

// VotePublicInputs binds a proposal id and a yes/no commitment for Vote kind.
// Commitment must equal ComputeVoteCommitment(nullifier, approve, nonce)
// for some (approve, nonce) the voter keeps secret until they later reveal
// it in a TxVoteReveal — a sealed-ballot commit-reveal on top of a real
// zero-knowledge eligibility proof (ShieldedTx.VoteEligibility): nullifier
// is that proof's own Nullifier, not a public identity. The TxVote's own
// Dilithium Sig/SignerPubKey still authenticate the transaction bytes
// against tampering, but — unlike this build's earlier design — need not
// be, and for the anonymity property below to mean anything must not be,
// the same keypair the voter's NFT was minted to; a wallet should sign
// with a fresh, unlinked key per vote. Only VoteEligibility ties a ballot
// to a real, minted NFT, and it does so without revealing which one (see
// VoteEligibilityProof's own doc).
type VotePublicInputs struct {
	ProposalID ID
	Commitment Hash

	// ParamKey/NewValue optionally bind this proposal to a concrete
	// governance.Params field change (spec 9.1/17.4), taking effect via
	// governance.ApplyParamChange once the proposal is tallied and
	// passes. They are only meaningful on the first TxVote to reference a
	// given ProposalID (see state.ProposalRecord's own doc: that first
	// vote is what creates the proposal record at all) — every
	// subsequent voter's own ParamKey/NewValue are ignored, so a later
	// voter cannot silently redefine what an earlier voter thought they
	// were voting on. Empty ParamKey means this proposal carries no
	// direct protocol effect (a plain up/down vote).
	ParamKey string
	NewValue string // decimal literal, e.g. "0.03" — see governance.ApplyParamChange
}

// VoteRevealPublicInputs opens a sealed TxVote ballot: Approve and Nonce
// must reproduce the Commitment the voter's earlier TxVote for the same
// ProposalID bound, via ComputeVoteCommitment. VoteEligibility is
// re-proved here too, not just at commit time (see pkg/tx's Stage 4 for
// why: the same nullifier must reappear, tying this reveal to the exact
// ballot its matching TxVote committed).
type VoteRevealPublicInputs struct {
	ProposalID ID
	Approve    bool
	Nonce      Hash
}

// VoteEligibilityProof is a real, anonymous zero-knowledge proof of spec
// 9.1's "one NFT, one vote": that the caster holds a leaf in the real
// eligibility-commitment tree (populated only by real, PoH-verified Kind
// NFTMint mints — see NFTMintPublicInputs.VoterCommitment) without
// revealing which leaf, bound to one specific proposal via Nullifier so
// the same NFT cannot cast two ballots on it while remaining free to vote
// on a different one (a different proposal yields an unlinkable
// nullifier — see VoteEligibilityScope). Proof is a real gnark Groth16
// proof over pkg/zk.EligibilityCircuit; pkg/zk.EligibilitySystem verifies
// it.
//
// This replaces this build's earlier plaintext SignerPubKey -> owner ->
// NFT lookup, which permanently tied every ballot to a public, long-lived
// wallet address. A real, disclosed trade-off of the anonymous design: a
// verifier can no longer tell whether the specific NFT behind a valid
// proof has since been slashed, since doing so would require learning
// which leaf voted — see pkg/tx's Stage 4 doc for the full disclosure.
type VoteEligibilityProof struct {
	Proof      []byte // gnark Groth16 proof bytes (pkg/zk.EligibilitySystem)
	MerkleRoot Hash   // claimed eligibility-tree root the proof anchors to
	// Nullifier is the proof's own per-(voter secret, proposal) output —
	// also the real per-proposal ballot dedup key (pkg/state.
	// ProposalRecord.Commitments/Reveals), replacing the old NFTID key.
	Nullifier Hash
}

// VoteEligibilityScope derives the public, proposal-scoping value a real
// VoteEligibilityProof's Nullifier is bound to (MiMC(VoterSK, scope) in
// pkg/zk.EligibilityCircuit). Both the prover (a wallet building a Vote/
// VoteReveal) and the verifier (pkg/tx's pipeline, which already has
// ProposalID on the tx) derive it identically from ProposalID alone, so
// it never needs to be transmitted as a separate tx field.
func VoteEligibilityScope(proposalID ID) Hash {
	return SumHash([]byte("shadowforge-vote-eligibility-scope-v1"), []byte(proposalID))
}

// ComputeVoteCommitment is this build's canonical sealed-ballot commit
// formula (see VotePublicInputs' doc: spec 17.4 names the commit step but
// not a concrete scheme). voter is the caster's real
// VoteEligibilityProof.Nullifier, not a public identity — binding it into
// the hash still ties a commitment to one specific (anonymous) voter, so
// a reveal can only open the ballot the same voter actually committed,
// not anyone else's, without that voter ever being named.
func ComputeVoteCommitment(voter Hash, approve bool, nonce Hash) Hash {
	var approveByte [1]byte
	if approve {
		approveByte[0] = 1
	}
	return SumHash(voter[:], approveByte[:], nonce[:])
}

// TraitPublicInputs binds the trait key and a delta commitment for NFTTrait
// kind.
type TraitPublicInputs struct {
	Key             string
	DeltaCommitment Hash
}

// NFTMintPublicInputs carries a real, signed proof-of-humanity
// attestation binding a TxNFTMint to one specific owner/mint attempt
// (spec 10.1). Fields mirror pkg/nft.PoHAttestation field-for-field
// rather than embedding it directly: pkg/nft already imports pkg/types,
// so the reverse import would cycle. pkg/tx's pipeline reconstructs a
// real pkg/nft.PoHAttestation from these fields and verifies it exactly
// the same way pkg/nft's own tests do.
type NFTMintPublicInputs struct {
	Owner Address
	// Nonce is a caller-chosen uniqueness salt for this mint attempt
	// (e.g. a wallet-local counter) — it must match Attestation's own
	// Nonce, binding one specific attestation to one specific mint
	// attempt so it can't be replayed against a different one.
	Nonce uint64
	// AttestationIssuedAtMs/Attestor/AttestationSig are the real signed
	// claim's fields (pkg/nft.PoHAttestation.IssuedAtMs/Attestor/Sig) —
	// Attestor is the attestor's real Dilithium public key, AttestationSig
	// its real signature over Hash(Owner, Nonce, AttestationIssuedAtMs).
	AttestationIssuedAtMs int64
	Attestor              []byte
	AttestationSig        DilithiumSig
	// VoterCommitment is the real, anonymous eligibility-tree leaf this
	// mint registers — MiMC(VoterSK) for a VoterSK the wallet derives
	// deterministically from its own signing key and never reveals
	// directly (zk.DeriveVoterSK/zk.VoterCommitment). pkg/tx's pipeline
	// inserts it into the real eligibility commitment tree at Stage 4,
	// the same way a Transfer's OutCommits are inserted into the note
	// tree; see VoteEligibilityProof for what later proves membership in
	// it without revealing which leaf. This field is public (any NFTMint
	// is already a public, PoH-attested event tying Owner to this leaf at
	// mint time) — the anonymity a later vote gets comes from the
	// membership proof never disclosing which past mint it corresponds
	// to, exactly as a shielded Transfer's later spend hides which past
	// output it spends even though that output's own creation was public.
	VoterCommitment Hash
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

// HashBlock computes a block's canonical header hash: everything a
// validator commits to before voting (Height, Epoch, PrevHash, Timestamp,
// TxRoot, StateRoot, DARoot, Proposer, DualTrack). Votes and ProposerSig
// are deliberately excluded — they are produced *over* this hash (a
// StageVote signs it; ProposerSig authenticates the proposal that led to
// it), so including them would be circular. Two blocks that differ only in
// which votes they happened to collect still hash identically, which is
// exactly what lets independently-computing validators agree on "the
// candidate" before votes exist. DualTrack, by contrast, is included: spec
// 5.6's outage-recovery bookkeeping (OutageController.RecordCleanDualTrackCycle,
// gating when OutageFlag may clear) is driven directly by it, so a relay
// must not be able to flip it on an already-signed block without
// invalidating every committee signature over the resulting hash.
func HashBlock(b Block) Hash {
	var heightBuf, epochBuf, tsBuf [8]byte
	for i := 0; i < 8; i++ {
		heightBuf[i] = byte(b.Height >> (8 * i))
		epochBuf[i] = byte(b.Epoch >> (8 * i))
		tsBuf[i] = byte(uint64(b.Timestamp) >> (8 * i))
	}
	dtBuf := [1]byte{0}
	if b.DualTrack {
		dtBuf[0] = 1
	}
	return SumHash(
		heightBuf[:], epochBuf[:], b.PrevHash[:], tsBuf[:],
		b.TxRoot[:], b.StateRoot[:], b.DARoot[:], b.Proposer[:], dtBuf[:],
	)
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
