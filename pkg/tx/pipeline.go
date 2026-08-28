package tx

import (
	"fmt"
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/container"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/staking"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// Deps are the modules the pipeline orchestrates. ZK may be nil only in
// tests that never submit a Kind Transfer transaction.
type Deps struct {
	// Store is state.Accessor, not *state.Store, so the pipeline can run
	// against either the real auto-committing Store (spec-compliant
	// single-node use, and every existing test) or a state.Txn held open
	// across a BFT vote round (pkg/validator) without any change to the
	// stage functions below.
	Store     state.Accessor
	StateTree *state.MerkleTree
	ZK        *zk.System
	// ZKTree is the real, canonical BN254/MiMC commitment tree every
	// honest node builds identically by inserting each committed
	// Transfer's real output commitments in the same order (Stage 4
	// below). Nil disables tree-membership bookkeeping entirely
	// (existing tests that never submit a Transfer keep working
	// unchanged).
	ZKTree *zk.Tree
	// ZKRoots is the historical root set Stage 1 checks a Transfer
	// proof's claimed MerkleRoot against — see zk.RootHistory's own doc
	// for the real gap this closes. Nil disables the check (existing
	// tests keep working unchanged); wiring it is what makes Kind
	// Transfer actually sound rather than merely internally consistent.
	ZKRoots *zk.RootHistory
	Vault   *vault.Vault
	// Silent is spec 15.4's per-wallet rate monitor. Nil disables spike
	// detection entirely (existing single-tx tests that don't care about
	// it keep working unchanged); when set, Stage 2 records every
	// signature-verified transaction against it and rejects one from a
	// wallet currently under a spike-response hold.
	Silent *silent.RateMonitor
	// Oracle is the real quorum-verified price/ATR feed (spec 11.3) Stage
	// 4 cross-checks a BankDeposit/BankWithdraw's claimed
	// OraclePriceUSD/ATRUSD against. Nil disables the cross-check entirely
	// (existing tests that bind an arbitrary self-consistent price keep
	// working unchanged) — without it, a BankDeposit/BankWithdraw's
	// internal buffer-vs-ATR consistency check alone accepts any claimed
	// price as long as it's self-consistent, which is exactly the real,
	// currently-exploitable gap wiring a real Oracle here closes.
	Oracle *oracle.Quorum
	// OracleTolerance bounds how far a claimed OraclePriceUSD/ATRUSD may
	// diverge from Oracle's real reading (a fraction, e.g. 0.02 == 2%)
	// before Stage 4 rejects the tx. Zero (the default) means
	// DefaultOracleTolerance.
	OracleTolerance decimal.Decimal
	// Governance is this node's live, per-node governance parameter set
	// (spec 9.1/17.4). Nil means Stage 4 falls back to pkg/bank's static
	// genesis-default ATR multiples (existing tests keep working
	// unchanged). When set, TallyDueProposals mutates it in place the
	// moment a ProposalParamChange proposal passes — the real effect a
	// governance vote passing has on the running protocol, closing the
	// gap where a proposal's tally outcome was persisted but never
	// actually changed anything live.
	Governance *governance.Params
	// Epoch is the epoch this batch belongs to (the block's own Epoch —
	// consensus-critical, since it's hashed into the block a committee
	// votes on). A new proposal a TxVote references for the first time is
	// stamped with this Epoch, anchoring when it's due for an
	// epoch-boundary tally.
	Epoch types.EpochNumber
	// Height is the height of the block this batch belongs to — stamped
	// onto a freshly minted ValidatorNFT's MintedAt (Kind NFTMint), the
	// same way Epoch is stamped onto a freshly created proposal record.
	Height types.BlockHeight
	// TrustedPoHAttestors is the currently-recognized proof-of-humanity
	// attestor public key set (spec 10.1) Kind NFTMint's real signed
	// attestation must be signed by one of. Unlike every other Deps
	// field above, nil here is NOT "check disabled" — it means no
	// attestor is trusted yet, so every NFTMint attempt is rejected
	// (fail closed): silently treating "not configured" as "skip the
	// check" would silently let anyone mint the very credential real
	// governance-Sybil-resistance (TxVote/TxVoteReveal below) now
	// depends on.
	TrustedPoHAttestors []crypto.DilithiumPublicKey
	// EligibilityZK, EligibilityTree, and EligibilityRoots are the real
	// anonymous-eligibility counterparts of ZK/ZKTree/ZKRoots above, for
	// the eligibility-commitment tree Kind NFTMint populates (Stage 4)
	// and TxVote/TxVoteReveal prove membership against
	// (requireEligibleVoterZK). Like TrustedPoHAttestors, and unlike
	// every ZK*/Silent/Oracle/Governance field above, EligibilityZK ==
	// nil is NOT "check disabled" — every TxVote/TxVoteReveal is
	// rejected fail-closed, since this is exactly what real
	// Sybil-resistance now depends on (a nil-tolerant "skip the check"
	// default here would silently let anyone vote). EligibilityTree/
	// EligibilityRoots must both be set whenever EligibilityZK is —
	// requireEligibleVoterZK treats a nil EligibilityRoots as a real
	// configuration error, not a disabled check, once EligibilityZK
	// itself is configured.
	EligibilityZK    *zk.EligibilitySystem
	EligibilityTree  *zk.Tree
	EligibilityRoots *zk.RootHistory
	// MintZK verifies a real spec-17.4 epoch-mint proof
	// (types.VotePublicInputs.MintProof) the moment a TxVote first binds
	// a proposal to a mint claim (Stage 4) — never lazily at tally time,
	// so a forged claim is rejected immediately rather than sitting in a
	// real proposal record un-caught until an epoch boundary. Nil is NOT
	// "mint checking disabled": a TxVote that actually carries a nonzero
	// MintAmount is rejected outright if MintZK is nil, for the identical
	// fail-closed reason EligibilityZK is — a nil-tolerant default here
	// would let anyone claim an unverified mint amount. A TxVote that
	// carries no mint claim at all (the overwhelming majority — ordinary
	// up/down votes and ParamKey changes) is entirely unaffected either
	// way.
	MintZK *zk.MintSystem
	// StakeZK verifies a real spec-17.4 "staked 2 percent yield" position-
	// creation proof (types.VotePublicInputs.StakeProof) the moment a
	// TxVote first binds a proposal to a staked mint claim (Stage 4) —
	// the staked-path counterpart of MintZK, checked at the identical
	// point in the identical fail-closed way (nil rejects only a TxVote
	// that actually carries MintStaked == true with a nonzero
	// MintAmount; every other TxVote is unaffected).
	StakeZK *zk.StakeSystem
	// StakeTree/StakeRoots are the real, canonical stake-position-
	// commitment tree and its historical root set — the staked-path
	// counterpart of ZKTree/ZKRoots, populated by TallyDueProposals (a
	// passed, staked mint proposal's real StakePositionCommit) rather
	// than by ordinary Stage 4 processing, since a position isn't known
	// to exist until its proposal actually passes. Nil (never true on a
	// real node) leaves a passed staked proposal's MintApplied false, the
	// same way a nil ZKTree does for the direct path.
	StakeTree  *zk.Tree
	StakeRoots *zk.RootHistory
	// UnstakeZK verifies a real Kind Unstake transaction's proof (Stage
	// 1, mirroring where Kind Transfer's own proof is checked — both are
	// real "spend a committed thing, mint a fresh output" claims, unlike
	// MintZK/StakeZK's claim-and-record shape checked at Stage 4). Nil
	// rejects every Kind Unstake transaction outright, fail-closed for
	// the identical reason ZK == nil rejects every Kind Transfer.
	UnstakeZK *zk.UnstakeSystem
	Now       func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// DefaultOracleTolerance is how far a BankDeposit/BankWithdraw's claimed
// OraclePriceUSD/ATRUSD may diverge from Deps.Oracle's real reading before
// Stage 4 rejects it, when Deps.OracleTolerance is left zero.
var DefaultOracleTolerance = decimal.MustFromString("0.02") // 2%

func (d Deps) oracleTolerance() decimal.Decimal {
	if d.OracleTolerance.Sign() <= 0 {
		return DefaultOracleTolerance
	}
	return d.OracleTolerance
}

// StageError reports which of the five stages (spec 5.3) rejected a
// transaction.
type StageError struct {
	Stage int
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("stage %d: %v", e.Stage, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

// Pipeline runs the five-stage validation sequence for a batch of
// transactions, enforcing the atomicity rule (spec 5.3): "if any stage
// fails, the whole transaction is rejected, pending nullifiers are
// released, and no trait or balance changes."
type Pipeline struct {
	deps Deps

	mu                sync.Mutex
	pendingNullifiers map[types.Hash]bool // Stage-1 reservations, not yet finalized
}

func NewPipeline(deps Deps) *Pipeline {
	return &Pipeline{deps: deps, pendingNullifiers: map[types.Hash]bool{}}
}

// Result is what ProcessBatch reports for one transaction.
type Result struct {
	Tx    types.ShieldedTx
	Error error // nil means all 5 stages committed
}

// ProcessBatch runs every entry through all five stages. Entries are
// processed independently: one transaction's rejection at any stage never
// blocks or corrupts another's, matching spec 5.3's per-transaction
// atomicity rule.
func (p *Pipeline) ProcessBatch(entries []Entry) []Result {
	results := make([]Result, 0, len(entries))
	for _, e := range entries {
		t := e.Tx
		err := p.processOne(&t, e.SubmittedAt)
		results = append(results, Result{Tx: t, Error: err})
	}
	return results
}

func (p *Pipeline) processOne(t *types.ShieldedTx, submittedAt time.Time) error {
	if err := p.stage1SenderLeave(t); err != nil {
		return &StageError{1, err}
	}
	if err := p.stage2TxOffer(t, submittedAt); err != nil {
		p.release(t)
		return &StageError{2, err}
	}
	if err := p.stage3ReceiverCheck(t); err != nil {
		p.release(t)
		return &StageError{3, err}
	}
	if err := p.stage4SendExec(t); err != nil {
		p.release(t)
		return &StageError{4, err}
	}
	if err := p.stage5PlaceFinal(t); err != nil {
		p.release(t)
		return &StageError{5, err}
	}
	// Stage 5 succeeded: finalize the nullifier reservations into real
	// spent records and drop them from the pending set.
	p.finalize(t)
	return nil
}

// --- Stage 1: Sender Leave ---
//
// "ZKP that the spent notes exist, are owned, and are not already
// nullified. Balance never revealed. Writes: Nullifier reserved in a
// pending set (not yet finalized)."
func (p *Pipeline) stage1SenderLeave(t *types.ShieldedTx) error {
	switch t.Kind {
	case types.TxTransfer:
		return p.stage1Transfer(t)
	case types.TxUnstake:
		return p.stage1Unstake(t)
	default:
		t.StageHints = t.StageHints.With(1)
		return nil
	}
}

func (p *Pipeline) stage1Transfer(t *types.ShieldedTx) error {
	pub := t.TransferPublicInputs
	if pub == nil || len(pub.Nullifiers) == 0 {
		return fmt.Errorf("transfer missing public inputs")
	}

	if err := p.reserveNullifiers(pub.Nullifiers); err != nil {
		return err
	}

	if p.deps.ZK == nil {
		return fmt.Errorf("no ZK system configured; cannot verify a Transfer proof")
	}

	rootElem := zk.FieldElementFromBytes32(pub.MerkleRoot)
	if p.deps.ZKRoots != nil && !p.deps.ZKRoots.Contains(rootElem) {
		// Checked before the expensive Groth16 verification below, both
		// because it's the cheaper check and because it's real DoS
		// resistance: a proof anchored to a root nobody ever produced is
		// rejected without spending CPU on a proof that was never going
		// to matter regardless of whether it verifies.
		return fmt.Errorf("proof anchored to an unrecognized commitment tree root")
	}

	zkPub := zk.TransferPublic{
		MerkleRoot: rootElem,
		Fee:        pub.FeeAmount,
	}
	for _, n := range pub.Nullifiers {
		zkPub.Nullifiers = append(zkPub.Nullifiers, zk.FieldElementFromBytes32(n))
	}
	for _, c := range pub.OutCommits {
		zkPub.OutCommits = append(zkPub.OutCommits, zk.FieldElementFromBytes32(c))
	}
	if err := p.deps.ZK.VerifyPublicProofBytes(t.Proof, zkPub); err != nil {
		return fmt.Errorf("zk proof invalid: %w", err)
	}

	p.markReserved(pub.Nullifiers)
	t.StageHints = t.StageHints.With(1)
	return nil
}

// stage1Unstake verifies a Kind Unstake transaction's real Groth16 proof
// at Stage 1, mirroring exactly where Kind Transfer's own proof is
// checked — both are "spend a real committed thing, mint a fresh output"
// claims (see types.TxUnstake's own doc for why this, not Stage 4, is the
// right place, unlike MintZK/StakeZK's claim-and-record checks).
func (p *Pipeline) stage1Unstake(t *types.ShieldedTx) error {
	pub := t.UnstakePublicInputs
	if pub == nil {
		return fmt.Errorf("unstake tx missing public inputs")
	}
	if len(t.Commitments) != 1 {
		return fmt.Errorf("unstake tx must carry exactly one output commitment, got %d", len(t.Commitments))
	}

	if err := p.reserveNullifiers([]types.Hash{t.Nullifier}); err != nil {
		return err
	}

	if p.deps.UnstakeZK == nil {
		return fmt.Errorf("no unstake ZK system configured; cannot verify an Unstake proof")
	}
	if p.deps.StakeRoots == nil {
		return fmt.Errorf("no stake root history configured; cannot verify an unstake proof anchors to a real tree")
	}

	rootElem := zk.FieldElementFromBytes32(pub.MerkleRoot)
	if !p.deps.StakeRoots.Contains(rootElem) {
		return fmt.Errorf("unstake proof anchored to an unrecognized stake-commitment tree root")
	}

	// The real yield formula is never trusted from the transaction's own
	// say-so: recompute it against this node's own current epoch and
	// reject outright if the claimed FinalAmount disagrees (pkg/staking's
	// own doc explains why StartEpoch itself is safe to trust here — it's
	// cryptographically pinned by the Merkle-membership check the proof
	// verification below performs).
	wantFinal := staking.FinalAmount(pub.Principal, pub.StartEpoch, p.deps.Epoch)
	if pub.FinalAmount != wantFinal {
		return fmt.Errorf("claimed final amount %d does not match the real yield formula for this epoch (want %d)", pub.FinalAmount, wantFinal)
	}

	unstakePub := zk.UnstakePublic{
		MerkleRoot:  rootElem,
		Nullifier:   zk.FieldElementFromBytes32(t.Nullifier),
		Principal:   pub.Principal,
		StartEpoch:  uint64(pub.StartEpoch),
		FinalAmount: pub.FinalAmount,
		OutCommit:   zk.FieldElementFromBytes32(t.Commitments[0]),
	}
	if err := p.deps.UnstakeZK.VerifyPublicProofBytes(t.Proof, unstakePub); err != nil {
		return fmt.Errorf("unstake proof invalid: %w", err)
	}

	p.markReserved([]types.Hash{t.Nullifier})
	t.StageHints = t.StageHints.With(1)
	return nil
}

// reserveNullifiers checks nullifiers against both this batch's in-flight
// reservations and the durably-spent set, without yet committing the
// reservation — shared by Transfer (possibly several nullifiers) and
// Unstake (always exactly one) so both get the identical double-spend
// protection.
func (p *Pipeline) reserveNullifiers(nullifiers []types.Hash) error {
	p.mu.Lock()
	for _, n := range nullifiers {
		if p.pendingNullifiers[n] {
			p.mu.Unlock()
			return fmt.Errorf("nullifier %s already reserved in this batch (double-spend)", n)
		}
	}
	p.mu.Unlock()

	for _, n := range nullifiers {
		spent, err := p.deps.Store.IsNullifierSpent(n)
		if err != nil {
			return fmt.Errorf("nullifier lookup: %w", err)
		}
		if spent {
			return fmt.Errorf("nullifier %s already spent", n)
		}
	}
	return nil
}

func (p *Pipeline) markReserved(nullifiers []types.Hash) {
	p.mu.Lock()
	for _, n := range nullifiers {
		p.pendingNullifiers[n] = true
	}
	p.mu.Unlock()
}

// txNullifiers returns the Stage-1 reservation(s) t implies, for release/
// finalize below — Transfer's plural TransferPublicInputs.Nullifiers, or
// Unstake's singular top-level Nullifier, or none for every other kind.
func txNullifiers(t *types.ShieldedTx) []types.Hash {
	switch t.Kind {
	case types.TxTransfer:
		if t.TransferPublicInputs == nil {
			return nil
		}
		return t.TransferPublicInputs.Nullifiers
	case types.TxUnstake:
		return []types.Hash{t.Nullifier}
	default:
		return nil
	}
}

// release drops a rejected transaction's Stage-1 reservations, per the
// atomicity rule.
func (p *Pipeline) release(t *types.ShieldedTx) {
	nullifiers := txNullifiers(t)
	if len(nullifiers) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, n := range nullifiers {
		delete(p.pendingNullifiers, n)
	}
}

func (p *Pipeline) finalize(t *types.ShieldedTx) {
	nullifiers := txNullifiers(t)
	if len(nullifiers) == 0 {
		return
	}
	p.mu.Lock()
	for _, n := range nullifiers {
		delete(p.pendingNullifiers, n)
	}
	p.mu.Unlock()
}

// --- Stage 2: TX Offer ---
//
// "Well-formedness: kind, fee commitment, circuit public inputs, Dilithium
// signature, not expired. Writes: Tx admitted to the working batch."
func (p *Pipeline) stage2TxOffer(t *types.ShieldedTx, submittedAt time.Time) error {
	if t.Kind > types.TxUnstake {
		return fmt.Errorf("unknown tx kind %d", t.Kind)
	}
	if !submittedAt.IsZero() && p.deps.now().Sub(submittedAt) > TxTTL {
		return fmt.Errorf("transaction expired (submitted %s ago, TTL %s)", p.deps.now().Sub(submittedAt), TxTTL)
	}
	// A single oversized transaction (an attacker-supplied huge Memo, for
	// instance) shouldn't be admissible at all, independent of
	// Mempool.DrainBatchBytes' batch-level size bound — that bound skips
	// including a tx that alone would blow the budget rather than
	// stalling, but a tx too large to EVER fit needs to be rejected here,
	// not perpetually deferred.
	if size, err := jsonSize(*t); err == nil && size > MaxTxSize {
		return fmt.Errorf("transaction is %d bytes serialized, exceeding the %d byte cap", size, MaxTxSize)
	}

	// Well-formedness: TxID must match its own content (spec 4.1), and the
	// Dilithium signature must actually verify against the signer's public
	// key (spec 5.3 Stage 2: "Dilithium signature"; spec 8.5: "Dilithium
	// signs: wallet authorizations..."). A non-empty Sig byte string alone
	// proves nothing — it must cryptographically bind to this exact TxID.
	wantTxID := types.ComputeTxID(t.Proof, t.Commitments, t.Nullifier)
	if t.TxID != wantTxID {
		return fmt.Errorf("TxID does not match Hash(proof || commitments || nullifier)")
	}
	if len(t.Sig) == 0 || len(t.SignerPubKey) == 0 {
		return fmt.Errorf("missing signature or signer public key")
	}
	ok, err := crypto.DilithiumVerify(crypto.DilithiumPublicKey(t.SignerPubKey), t.TxID[:], crypto.DilithiumSignature(t.Sig))
	if err != nil {
		return fmt.Errorf("signature check: %w", err)
	}
	if !ok {
		return fmt.Errorf("signature does not verify against the claimed signer key")
	}

	// Spike detection (spec 15.4) keys off the wallet identity the
	// signature check above just proved genuine — never off an unverified
	// claimed pubkey, which an attacker could pick freely to paint an
	// arbitrary victim address as spiking. This must run after the
	// DilithiumVerify above, not before it.
	if p.deps.Silent != nil {
		now := p.deps.now()
		addr := types.Address(types.SumHash(t.SignerPubKey))
		if p.deps.Silent.IsHeld(addr, now) {
			return fmt.Errorf("wallet is under an active spike-response hold")
		}
		p.deps.Silent.RecordTx(addr, now)
		if action, flagged := silent.EvaluateSpike(p.deps.Silent, addr, now); flagged {
			p.deps.Silent.PlaceHold(addr, action.HoldUntil)
			return fmt.Errorf("transaction rate spike detected: wallet held until %s", action.HoldUntil)
		}
	}

	switch t.Kind {
	case types.TxTransfer:
		if t.FeeCommit.IsZero() {
			return fmt.Errorf("transfer missing fee commitment")
		}
	case types.TxBankDeposit, types.TxBankWithdraw:
		if t.BankPublicInputs == nil {
			return fmt.Errorf("bank tx missing public inputs")
		}
	case types.TxVote:
		if t.VotePublicInputs == nil {
			return fmt.Errorf("vote tx missing public inputs")
		}
		if t.VoteEligibility == nil {
			return fmt.Errorf("vote tx missing real anonymous eligibility proof (VoteEligibility)")
		}
		if t.VotePublicInputs.MintAmount != 0 {
			if t.VotePublicInputs.MintStaked {
				if len(t.VotePublicInputs.StakeProof) == 0 {
					return fmt.Errorf("vote tx claims a staked mint amount but is missing StakeProof")
				}
				if t.VotePublicInputs.StakePositionCommit.IsZero() {
					return fmt.Errorf("vote tx claims a staked mint amount but StakePositionCommit is empty")
				}
			} else {
				if len(t.VotePublicInputs.MintProof) == 0 {
					return fmt.Errorf("vote tx claims a mint amount but is missing MintProof")
				}
				if t.VotePublicInputs.MintOutCommit.IsZero() {
					return fmt.Errorf("vote tx claims a mint amount but MintOutCommit is empty")
				}
			}
		}
	case types.TxVoteReveal:
		if t.VoteRevealPublicInputs == nil {
			return fmt.Errorf("vote reveal tx missing public inputs")
		}
		if t.VoteEligibility == nil {
			return fmt.Errorf("vote reveal tx missing real anonymous eligibility proof (VoteEligibility)")
		}
	case types.TxNFTTrait:
		if t.TraitPublicInputs == nil {
			return fmt.Errorf("NFTTrait tx missing public inputs")
		}
	case types.TxNFTMint:
		if t.NFTMintPublicInputs == nil {
			return fmt.Errorf("NFTMint tx missing public inputs")
		}
	case types.TxUnstake:
		if t.UnstakePublicInputs == nil {
			return fmt.Errorf("unstake tx missing public inputs")
		}
		if len(t.Proof) == 0 {
			return fmt.Errorf("unstake tx missing proof")
		}
		if t.Nullifier.IsZero() {
			return fmt.Errorf("unstake tx missing nullifier")
		}
	}
	t.StageHints = t.StageHints.With(2)
	return nil
}

// --- Stage 3: Receiver Check ---
//
// "Receiver note parameters legal; optional compliance hook (KYC oracle
// flag ...); container routing if ContainerID set. Writes: Receiver
// pending note."
func (p *Pipeline) stage3ReceiverCheck(t *types.ShieldedTx) error {
	if t.Kind == types.TxTransfer {
		if t.TransferPublicInputs == nil || len(t.TransferPublicInputs.OutCommits) == 0 {
			return fmt.Errorf("transfer has no receiver note commitments")
		}
	}
	if t.ContainerID != nil {
		// Container routing: the transaction is destined for (or
		// originated in) an enterprise subspace. Shadow verification
		// itself happens at Stage 4/5 via container.ShadowVerify; this
		// stage only confirms the container reference is well-formed.
		if *t.ContainerID == "" {
			return fmt.Errorf("empty container id")
		}
	}
	t.StageHints = t.StageHints.With(3)
	return nil
}

// --- Stage 4: Send Exec ---
//
// "Atomic application of the state transition against a working copy of
// the Merkle tree. Bank and mint kinds run their extra math here. Writes:
// Working state root candidate."
func (p *Pipeline) stage4SendExec(t *types.ShieldedTx) error {
	switch t.Kind {
	case types.TxTransfer:
		outCommits := t.TransferPublicInputs.OutCommits
		if p.deps.ZKTree != nil && p.deps.ZKTree.Remaining() < len(outCommits) {
			// Checked before any mutation (of StateTree or ZKTree) so a
			// transaction whose outputs wouldn't all fit is rejected
			// outright, per the atomicity rule, rather than leaving a
			// partially-inserted commitment behind for a transaction
			// that ultimately fails.
			return fmt.Errorf("canonical commitment tree has no room for %d more output commitment(s)", len(outCommits))
		}
		for _, c := range outCommits {
			p.deps.StateTree.Append(c)
		}
		if p.deps.ZKTree != nil {
			for _, c := range outCommits {
				if _, err := p.deps.ZKTree.Insert(zk.FieldElementFromBytes32(c)); err != nil {
					// Remaining() was already checked above under this
					// same call's single-threaded processing, so reaching
					// here would mean that invariant broke somewhere —
					// fail loudly rather than silently proceed.
					return fmt.Errorf("insert output commitment into canonical tree: %w", err)
				}
			}
			if p.deps.ZKRoots != nil {
				newRoot, err := p.deps.ZKTree.Root()
				if err != nil {
					return fmt.Errorf("compute canonical tree root: %w", err)
				}
				p.deps.ZKRoots.Record(newRoot)
			}
		}

	case types.TxBankDeposit, types.TxBankWithdraw:
		// Defense-in-depth: recompute the buffer/retention relationship
		// the tx's bound public inputs claim and reject if it doesn't
		// match the spec 19.3/19.4 formula (pkg/bank owns the full
		// deposit/withdraw math off the shielded path; this only checks
		// the ATR-derived figure bound into the proof's public inputs is
		// internally consistent before any state changes).
		pub := t.BankPublicInputs
		depositMult, withdrawMult := bank.DepositATRMultiple, bank.WithdrawATRMultiple
		if p.deps.Governance != nil {
			// A passed ProposalParamChange vote (TallyDueProposals) has
			// already mutated Deps.Governance in place — reading it here
			// live, rather than the static pkg/bank defaults, is what
			// makes that vote's effect real for every subsequent
			// BankDeposit/BankWithdraw rather than just persisted trivia.
			depositMult, withdrawMult = p.deps.Governance.DepositATRMultiple, p.deps.Governance.WithdrawATRMultiple
		}
		var wantBuffer decimal.Decimal
		if t.Kind == types.TxBankDeposit {
			wantBuffer = depositMult.Mul(pub.ATRUSD)
		} else {
			wantBuffer = withdrawMult.Mul(pub.ATRUSD)
		}
		if pub.BufferUSD.Cmp(wantBuffer) != 0 {
			return fmt.Errorf("bound buffer %s does not match %s * ATR (%s)", pub.BufferUSD, t.Kind, wantBuffer)
		}
		if p.deps.Oracle != nil {
			if err := p.checkOraclePrice(t.Kind, pub); err != nil {
				return err
			}
		}
		for _, c := range t.Commitments {
			p.deps.StateTree.Append(c)
		}

	case types.TxNFTTrait:
		// The trait delta itself is shielded (TraitPublicInputs.DeltaCommitment
		// is a commitment, not a plaintext value), so Stage 4 can only
		// confirm the target NFT exists and record that a committed
		// update was applied to it — decrypting and applying the actual
		// numeric delta requires the receiver's viewing key and happens
		// client-side, mirroring how a shielded Transfer's amount is
		// never visible to the pipeline either.
		if len(t.Commitments) == 0 {
			return fmt.Errorf("NFTTrait tx missing target NFT reference")
		}
		targetID := types.NFTID(t.Commitments[0])
		nftRec, found, err := p.deps.Store.GetNFT(targetID)
		if err != nil {
			return fmt.Errorf("nft lookup: %w", err)
		}
		if !found {
			return fmt.Errorf("no NFT record for trait update target %s", targetID)
		}
		if nftRec.Traits == nil {
			nftRec.Traits = map[string]string{}
		}
		nftRec.Traits[t.TraitPublicInputs.Key+"_last_delta_commitment"] = t.TraitPublicInputs.DeltaCommitment.String()
		if err := p.deps.Store.PutNFT(nftRec); err != nil {
			return fmt.Errorf("persist nft: %w", err)
		}

	case types.TxVote:
		// Records this sealed ballot's commitment against its proposal,
		// keyed by voter (spec 17.4: "Votes accumulate as ZKP ballots
		// during the epoch"). The choice itself stays hidden until a
		// later TxVoteReveal opens it; a real epoch-boundary tally
		// (pkg/validator, once this proposal's Epoch has passed) counts
		// only what got revealed — see types.ComputeVoteCommitment and
		// the TxVoteReveal case below.
		//
		// Real, anonymous voter eligibility (spec 9.1's "one NFT, one
		// vote"): requireEligibleVoterZK verifies a real ZK proof that
		// the caster holds a minted NFT without learning which one — see
		// its own doc. voter is that proof's own Nullifier, which is
		// what dedup below keys off (not a name), and it is also what
		// ComputeVoteCommitment binds instead of a public identity.
		voter, err := p.requireEligibleVoterZK(t.VoteEligibility, t.VotePublicInputs.ProposalID)
		if err != nil {
			return err
		}
		pub := t.VotePublicInputs
		record, found, err := p.deps.Store.GetProposal(string(pub.ProposalID))
		if err != nil {
			return fmt.Errorf("proposal lookup: %w", err)
		}
		if !found {
			// Real epoch-mint claim (spec 17.4), checked once, right
			// here, at the exact point its claim becomes durable — not
			// lazily at tally time — for the same reason every other
			// real proof in this pipeline is checked at submission, not
			// deferred: a forged claim must never be allowed to sit in a
			// real proposal record un-caught. Only the first vote to
			// reference this ProposalID binds what it does (types.
			// VotePublicInputs' own doc) — a later voter's own MintAmount/
			// MintOutCommit/MintProof are never consulted.
			if pub.MintAmount != 0 {
				if pub.MintStaked {
					if p.deps.StakeZK == nil {
						return fmt.Errorf("no stake ZK system configured; cannot verify a real staked epoch-mint claim")
					}
					// StartEpoch is not a free choice: it must be exactly
					// this proposal's own creation epoch, closing off a
					// proposer picking an earlier epoch to front-run extra
					// yield (pkg/staking's own doc).
					stakePub := zk.StakePublic{
						Principal:      pub.MintAmount,
						StartEpoch:     uint64(p.deps.Epoch),
						PositionCommit: zk.FieldElementFromBytes32(pub.StakePositionCommit),
					}
					if err := p.deps.StakeZK.VerifyPublicProofBytes(pub.StakeProof, stakePub); err != nil {
						return fmt.Errorf("stake proof invalid: %w", err)
					}
				} else {
					if p.deps.MintZK == nil {
						return fmt.Errorf("no mint ZK system configured; cannot verify a real epoch-mint claim")
					}
					mintPub := zk.MintPublic{
						Amount:    types.MintNetAmount(pub.MintAmount),
						OutCommit: zk.FieldElementFromBytes32(pub.MintOutCommit),
					}
					if err := p.deps.MintZK.VerifyPublicProofBytes(pub.MintProof, mintPub); err != nil {
						return fmt.Errorf("mint proof invalid: %w", err)
					}
				}
			}
			if !pub.SlashTargetNFT.IsZero() {
				// A slash proposal against a nonexistent (or already
				// burned) NFT can never even be created — checked once,
				// here, at the exact point the claim becomes durable, for
				// the same reason every other real claim in this pipeline
				// is checked at submission rather than deferred to tally.
				if _, found, err := p.deps.Store.GetNFT(pub.SlashTargetNFT); err != nil {
					return fmt.Errorf("slash target lookup: %w", err)
				} else if !found {
					return fmt.Errorf("slash proposal targets a nonexistent NFT %s", pub.SlashTargetNFT)
				}
			}
			record = state.ProposalRecord{
				ProposalID:  string(pub.ProposalID),
				Epoch:       p.deps.Epoch,
				Commitments: map[types.Hash]types.Hash{},
				Reveals:     map[types.Hash]bool{},
				// Only the first vote to reference this ProposalID binds
				// what it does (types.VotePublicInputs' own doc) — a
				// later voter's own ParamKey/NewValue are never
				// consulted, since record is only mutated here on
				// creation.
				ParamKey:            pub.ParamKey,
				NewValue:            pub.NewValue,
				MintAmount:          pub.MintAmount,
				MintOutCommit:       pub.MintOutCommit,
				MintStaked:          pub.MintStaked,
				StakePositionCommit: pub.StakePositionCommit,
				SlashTargetNFT:      pub.SlashTargetNFT,
				SlashBurn:           pub.SlashBurn,
			}
		}
		if record.Tallied {
			return fmt.Errorf("proposal %s already tallied; voting is closed", pub.ProposalID)
		}
		if _, already := record.Commitments[voter]; already {
			return fmt.Errorf("a ballot with this nullifier has already been cast for proposal %s (double-vote)", pub.ProposalID)
		}
		record.Commitments[voter] = pub.Commitment
		if err := p.deps.Store.PutProposal(record); err != nil {
			return fmt.Errorf("persist proposal: %w", err)
		}

	case types.TxVoteReveal:
		// Opens a sealed TxVote ballot: Approve/Nonce must reproduce the
		// Commitment that voter's earlier TxVote bound, or the reveal is
		// rejected outright — this is a real cryptographic check against
		// stored on-chain state, not a trust-the-caller flag.
		// Eligibility is re-proved here too (not just at commit time):
		// the reveal must carry a valid proof for the same nullifier the
		// matching TxVote committed under, or it cannot open that
		// ballot at all (see the Commitments lookup below).
		voter, err := p.requireEligibleVoterZK(t.VoteEligibility, t.VoteRevealPublicInputs.ProposalID)
		if err != nil {
			return err
		}
		pub := t.VoteRevealPublicInputs
		record, found, err := p.deps.Store.GetProposal(string(pub.ProposalID))
		if err != nil {
			return fmt.Errorf("proposal lookup: %w", err)
		}
		if !found {
			return fmt.Errorf("no such proposal %s", pub.ProposalID)
		}
		if record.Tallied {
			return fmt.Errorf("proposal %s already tallied; reveals are closed", pub.ProposalID)
		}
		commitment, committed := record.Commitments[voter]
		if !committed {
			return fmt.Errorf("no ballot with this nullifier has been cast for proposal %s", pub.ProposalID)
		}
		if record.Reveals[voter] {
			return fmt.Errorf("a ballot with this nullifier has already been revealed for proposal %s", pub.ProposalID)
		}
		if want := types.ComputeVoteCommitment(voter, pub.Approve, pub.Nonce); want != commitment {
			return fmt.Errorf("reveal does not match the earlier commitment for proposal %s", pub.ProposalID)
		}
		record.Reveals[voter] = pub.Approve
		if err := p.deps.Store.PutProposal(record); err != nil {
			return fmt.Errorf("persist proposal: %w", err)
		}

	case types.TxMint:
		// Not the real spec-17.4 epoch-mint mechanism — see types.
		// TxMint's own doc. Accepted (well-formedness already checked at
		// Stage 2) but genuinely has no effect; the real mechanism is the
		// TxVote case above, when VotePublicInputs.MintAmount != 0.

	case types.TxContainerSync:
		if t.ContainerID == nil || len(t.Commitments) == 0 {
			return fmt.Errorf("ContainerSync tx missing container id or sync root")
		}
		newRoot := t.Commitments[0]
		if _, found, err := p.deps.Store.GetContainerRoot(string(*t.ContainerID)); err != nil {
			return fmt.Errorf("container root lookup: %w", err)
		} else if found {
			// Shadow verification (spec 16.3): "compares the container
			// output to the duplicate server. Mismatch blocks commit" —
			// unconditional once a prior root exists, so a real
			// integration's duplicate-server digest (the tx's Memo) must
			// actually be present here, not just checked when a
			// submitter happens to include one. Requiring it only from
			// the second sync onward (not the first, which has nothing
			// yet to shadow-compare against) closes a real bypass: a
			// submitter who simply omitted the memo previously skipped
			// verification entirely instead of being rejected.
			if len(t.Memo) != 32 {
				return fmt.Errorf("shadow verification required: expected a 32-byte duplicate-server digest in Memo, got %d bytes", len(t.Memo))
			}
			var shadow types.Hash
			copy(shadow[:], t.Memo)
			if !container.ShadowVerify(newRoot, shadow) {
				return fmt.Errorf("shadow verification mismatch: container output does not match duplicate server")
			}
		}
		if err := p.deps.Store.PutContainerRoot(string(*t.ContainerID), newRoot); err != nil {
			return fmt.Errorf("persist container root: %w", err)
		}

	case types.TxNFTMint:
		pub := t.NFTMintPublicInputs
		_, alreadyHasNFT, err := p.deps.Store.GetNFTByOwner(pub.Owner)
		if err != nil {
			return fmt.Errorf("nft-by-owner lookup: %w", err)
		}
		minted, err := nft.Mint(nft.MintParams{
			Owner:         pub.Owner,
			AlreadyHasNFT: alreadyHasNFT,
			Attestation: nft.PoHAttestation{
				Owner:      pub.Owner,
				Nonce:      pub.Nonce,
				IssuedAtMs: pub.AttestationIssuedAtMs,
				Attestor:   crypto.DilithiumPublicKey(pub.Attestor),
				Sig:        crypto.DilithiumSignature(pub.AttestationSig),
			},
			TrustedAttestors: p.deps.TrustedPoHAttestors,
			MintedAt:         p.deps.Height,
			Now:              p.deps.now(),
			Nonce:            pub.Nonce,
		})
		if err != nil {
			return fmt.Errorf("nft mint: %w", err)
		}
		if err := p.deps.Store.PutNFT(minted); err != nil {
			return fmt.Errorf("persist minted nft: %w", err)
		}
		// Real anonymous voter eligibility (VoteEligibilityProof): insert
		// this mint's VoterCommitment into the same real, canonical
		// commitment tree TxVote/TxVoteReveal later prove membership
		// against, exactly mirroring how Kind Transfer's OutCommits are
		// inserted into ZKTree above — every honest node builds an
		// identical EligibilityTree by processing NFTMints in the same
		// committed order.
		if p.deps.EligibilityTree != nil {
			if _, err := p.deps.EligibilityTree.Insert(zk.FieldElementFromBytes32(pub.VoterCommitment)); err != nil {
				return fmt.Errorf("insert voter commitment into eligibility tree: %w", err)
			}
			if p.deps.EligibilityRoots != nil {
				newRoot, err := p.deps.EligibilityTree.Root()
				if err != nil {
					return fmt.Errorf("compute eligibility tree root: %w", err)
				}
				p.deps.EligibilityRoots.Record(newRoot)
			}
		}

	case types.TxUnstake:
		// The real proof (membership of the redeemed position, correct
		// nullifier, correct binding of FinalAmount to the new note) was
		// already verified once, at Stage 1 — this only makes the
		// resulting note real by inserting it into the same canonical
		// tree a Transfer's own outputs live in, mirroring exactly how
		// Transfer's own OutCommits are inserted above.
		outCommit := t.Commitments[0]
		if p.deps.ZKTree != nil {
			if p.deps.ZKTree.Remaining() < 1 {
				return fmt.Errorf("canonical commitment tree has no room for the unstake proceeds note")
			}
			if _, err := p.deps.ZKTree.Insert(zk.FieldElementFromBytes32(outCommit)); err != nil {
				return fmt.Errorf("insert unstake proceeds commitment into canonical tree: %w", err)
			}
			p.deps.StateTree.Append(outCommit)
			if p.deps.ZKRoots != nil {
				newRoot, err := p.deps.ZKTree.Root()
				if err != nil {
					return fmt.Errorf("compute canonical tree root: %w", err)
				}
				p.deps.ZKRoots.Record(newRoot)
			}
		}
	}

	t.StageHints = t.StageHints.With(4)
	return nil
}

// requireEligibleVoterZK enforces spec 9.1's "one NFT, one vote"
// anonymously: it verifies a real ZK proof that the caster holds some
// real, minted NFT (a leaf in Deps.EligibilityTree, populated only by
// real Kind NFTMint mints — see that Stage 4 case) without ever learning
// which one. This is the anonymous replacement for this build's earlier
// requireEligibleVoter, which resolved SignerPubKey to a real owner
// address and looked up that owner's NFT directly — sound, but a
// permanent public link between every ballot and a long-lived wallet
// identity. Proving membership this way instead means an observer of a
// Vote/VoteReveal transaction learns nothing about which NFT cast it,
// only that a real one did.
//
// On success it returns the proof's own Nullifier — what the
// TxVote/TxVoteReveal cases above key ballot commit/dedup off, instead of
// a named voter. Nullifier is scoped to proposalID (types.
// VoteEligibilityScope), so the same real NFT proves a distinct,
// unlinkable nullifier for each different proposal it votes on, while
// reproducing the identical one for a second attempt on the same
// proposal — that reproduction, not a lookup by identity, is what makes
// double-voting on one proposal actually impossible.
//
// A real, disclosed limitation of this anonymous design, replacing the
// old check's rec.Slashed test: because a valid proof never reveals
// which leaf it opens, this cannot tell whether that specific NFT has
// since been slashed — even though slash execution itself is now real
// (types.VotePublicInputs.SlashTargetNFT; TallyDueProposals calls
// pkg/nft.ApplySlash on a pass), the slashed NFT's own record being
// really updated doesn't help a membership proof that never names which
// leaf it is. Revoking an anonymous credential needs either a
// slashed-leaf accumulator or epoch-scoped re-registration, and this
// build implements neither yet.
func (p *Pipeline) requireEligibleVoterZK(proof *types.VoteEligibilityProof, proposalID types.ID) (types.Hash, error) {
	if p.deps.EligibilityZK == nil {
		return types.Hash{}, fmt.Errorf("no anonymous eligibility ZK system configured; cannot verify a voter eligibility proof")
	}
	if p.deps.EligibilityRoots == nil {
		return types.Hash{}, fmt.Errorf("no eligibility root history configured; cannot verify a voter eligibility proof anchors to a real tree")
	}
	if proof == nil {
		return types.Hash{}, fmt.Errorf("missing voter eligibility proof")
	}

	rootElem := zk.FieldElementFromBytes32(proof.MerkleRoot)
	if !p.deps.EligibilityRoots.Contains(rootElem) {
		// Checked before the expensive Groth16 verification below, for
		// the same real DoS-resistance and soundness reasons Stage 1's
		// Transfer root check documents: a proof anchored to a root
		// nobody ever produced is rejected without spending CPU on a
		// proof that was never going to matter, and it closes the
		// analogous gap a bare "internally consistent" check alone would
		// leave (a self-invented tree with a self-invented member).
		return types.Hash{}, fmt.Errorf("eligibility proof anchored to an unrecognized commitment tree root")
	}

	scopeElem := zk.FieldElementFromBytes32(types.VoteEligibilityScope(proposalID))
	nullifierElem := zk.FieldElementFromBytes32(proof.Nullifier)
	pub := zk.EligibilityPublic{
		MerkleRoot:    rootElem,
		Nullifier:     nullifierElem,
		ProposalScope: scopeElem,
	}
	if err := p.deps.EligibilityZK.VerifyPublicProofBytes(proof.Proof, pub); err != nil {
		return types.Hash{}, fmt.Errorf("eligibility proof invalid: %w", err)
	}
	return proof.Nullifier, nil
}

// checkOraclePrice cross-checks a BankDeposit/BankWithdraw's claimed
// OraclePriceUSD/ATRUSD against p.deps.Oracle's real quorum-verified
// reading, implementing spec 11.3's disagreement policy: a deposit is
// frozen outright if the real oracle itself can't produce an agreed
// reading (ErrDisagreement or every source failing), while a withdrawal
// falls back to the last-agreed snapshot so an open hold can still be
// closed out during a freeze. Either way, once a real reading is in hand,
// the claimed figures must sit within Deps.oracleTolerance() of it — the
// internal buffer-vs-ATR consistency check above only proves the tx is
// self-consistent, not that the price it's self-consistent about is real.
func (p *Pipeline) checkOraclePrice(kind types.TxKind, pub *types.BankPublicInputs) error {
	quote, err := p.deps.Oracle.Quote(pub.Asset)
	if err != nil {
		if kind != types.TxBankWithdraw {
			return fmt.Errorf("oracle unavailable for %s, freezing new deposits: %w", pub.Asset, err)
		}
		lastGood, ok := p.deps.Oracle.LastGood(pub.Asset)
		if !ok {
			return fmt.Errorf("oracle unavailable and no last-good snapshot for %s: %w", pub.Asset, err)
		}
		quote = lastGood
	}
	tol := p.deps.oracleTolerance()
	if !withinTolerance(pub.OraclePriceUSD, quote.PriceUSD, tol) {
		return fmt.Errorf("claimed price %s for %s diverges from oracle reading %s beyond tolerance", pub.OraclePriceUSD, pub.Asset, quote.PriceUSD)
	}
	if !withinTolerance(pub.ATRUSD, quote.ATRUSD, tol) {
		return fmt.Errorf("claimed ATR %s for %s diverges from oracle reading %s beyond tolerance", pub.ATRUSD, pub.Asset, quote.ATRUSD)
	}
	return nil
}

// withinTolerance reports whether claimed is within the fractional bound
// tol of real (e.g. tol=0.02 means claimed must sit within 2% of real).
// real<=0 only passes when claimed is also exactly zero, mirroring
// pkg/oracle's own withinBound treatment of a zero reference price.
func withinTolerance(claimed, real, tol decimal.Decimal) bool {
	if real.Sign() <= 0 {
		return claimed.Sign() == 0
	}
	diff := claimed.Sub(real)
	if diff.IsNeg() {
		diff = decimal.Zero.Sub(diff)
	}
	return diff.Div(real).Cmp(tol) <= 0
}

// --- Stage 5: Place Final ---
//
// "BFT votes from assigned validators; commit state root, DA root, burn
// ephemeral pipeline artifacts. Writes: Block inclusion, pending ->
// committed."
func (p *Pipeline) stage5PlaceFinal(t *types.ShieldedTx) error {
	if t.Kind == types.TxTransfer {
		for _, n := range t.TransferPublicInputs.Nullifiers {
			if err := p.deps.Store.MarkNullifierSpent(n); err != nil {
				return fmt.Errorf("commit nullifier: %w", err)
			}
		}
		// FeeAmount is a public input the ZK proof already bound and Stage
		// 1 already verified (value conservation: inputs = outputs + fee),
		// so this is real, cryptographically-proven revenue, not a
		// fabricated figure — route it into the Vault's 20/10/10/60 split
		// (spec 9.2) now that every stage has committed.
		if p.deps.Vault != nil {
			p.deps.Vault.CollectFee(decimal.FromInt(int64(t.TransferPublicInputs.FeeAmount)))
		}
	}
	if t.Kind == types.TxUnstake {
		// Durably marks the redeemed position's nullifier spent, in the
		// same shared nullifier-spent set an ordinary note's own
		// nullifier uses (types.ShieldedTx.Nullifier's own doc) — no
		// staked position can ever be redeemed twice.
		if err := p.deps.Store.MarkNullifierSpent(t.Nullifier); err != nil {
			return fmt.Errorf("commit unstake nullifier: %w", err)
		}
	}
	t.StageHints = t.StageHints.With(5)
	return nil
}

// TallyDueProposals runs spec 17.4's epoch-end tally for every untallied
// proposal whose Epoch is strictly before currentEpoch: real revealed
// ballots (see the TxVote/TxVoteReveal cases above) are counted via
// governance.Tally, and the outcome is persisted so the proposal never
// gets tallied twice. A passed proposal that's bound to a ParamKey (see
// state.ProposalRecord's own doc) has that change actually applied to
// Deps.Governance — and, for a Vault allocation share, resynced into
// Deps.Vault.Splits too — the real effect a governance vote passing has
// on the running protocol (closing the gap where a tally's outcome was
// persisted but never changed anything live); Deps.Governance == nil
// skips this step entirely (existing tests that don't wire it up keep
// working unchanged). A passed proposal bound to a real MintAmount (spec
// 17.4 — see types.VotePublicInputs.MintAmount's own doc) really does
// mint SFG now, on whichever of the two real proposer paths it
// requested: the direct path inserts MintOutCommit — already
// proof-verified once, at Stage 4, when the first vote bound it — into
// the same canonical tree a Transfer's own outputs live in, becoming a
// real, spendable note, and the Vault collects its real fee share
// (types.MintFeeAmount); the staked path (pkg/staking's own doc has the
// real yield formula) instead inserts StakePositionCommit into the
// separate, real canonical stake tree, with no Vault fee taken (a real,
// deliberate asymmetry between the two paths, not an oversight) — a
// later Kind Unstake transaction is what actually pays it out. Deps.
// ZKTree/StakeTree == nil (never true on a real node) skips the
// relevant step the same way Deps.Governance == nil skips ParamKey
// application. A passed proposal bound to a real SlashTargetNFT (spec
// 10.3 — see types.VotePublicInputs.SlashTargetNFT's own doc) really
// does slash the target NFT now: pkg/nft.ApplySlash freezes it
// (Slashed=true, record kept) or, for the burn outcome, the record is
// permanently removed (Store.DeleteNFT) — the target's real existence
// was already checked once, at Stage 4. unlock-transfer/container-asset
// proposal kinds are tallied like any other but have no execution wired
// here either — see governance.ProposalKind's own doc for the
// remaining kinds this build does not yet apply.
//
// This runs deterministically off already-committed proposal state plus
// the caller's own block Epoch, so every honest node reaches the same
// result when processing the same block content — it does not depend on
// wall-clock time or on which reveals *this* node happens to have seen
// outside what the chain itself has committed.
func (p *Pipeline) TallyDueProposals(currentEpoch types.EpochNumber) ([]state.ProposalRecord, error) {
	all, err := p.deps.Store.ListProposals()
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	eligibleNFTs, err := p.deps.Store.CountNFTs()
	if err != nil {
		return nil, fmt.Errorf("count nfts: %w", err)
	}

	var tallied []state.ProposalRecord
	for _, record := range all {
		if record.Tallied || record.Epoch >= currentEpoch {
			continue
		}
		ballots := make([]governance.Ballot, 0, len(record.Reveals))
		for voter, approve := range record.Reveals {
			ballots = append(ballots, governance.Ballot{Voter: voter, Approve: approve})
		}
		result := governance.Tally(ballots, eligibleNFTs, governance.MinTurnout)

		record.Tallied = true
		record.Approve = result.Approve
		record.Reject = result.Reject
		record.Passed = result.Passed

		if record.Passed && record.ParamKey != "" && p.deps.Governance != nil {
			// A malformed/unrecognized claim from whoever cast the first
			// vote must not block tallying every other due proposal in
			// this same call — it only leaves this one's own Applied
			// false, which is the durable, auditable signal that the
			// vote passed but its bound change never took effect.
			if err := governance.ApplyParamChange(p.deps.Governance, record.ParamKey, record.NewValue); err == nil {
				record.Applied = true
				if governance.IsVaultShareKey(record.ParamKey) && p.deps.Vault != nil {
					p.deps.Vault.Splits = vault.SplitsFromParams(*p.deps.Governance)
				}
			}
		}

		if record.Passed && record.MintAmount > 0 && record.MintStaked {
			// Real spec-17.4 staked-yield mint execution: StakePositionCommit's
			// real Groth16 proof was already verified once, at Stage 4 —
			// this only makes the locked position real by inserting it into
			// the real, canonical stake-commitment tree, mirroring the
			// direct path's own ZKTree insertion below, but into a
			// separate tree since the position is not yet spendable (it
			// becomes so only once a later TxUnstake redeems it). No Vault
			// fee is collected on this path — see pkg/staking's own doc for
			// why that asymmetry with the direct path is deliberate.
			// StakeTree == nil (never true on a real node) leaves
			// MintApplied false, mirroring the direct path's identical
			// treatment.
			if p.deps.StakeTree != nil {
				if _, err := p.deps.StakeTree.Insert(zk.FieldElementFromBytes32(record.StakePositionCommit)); err == nil {
					record.MintApplied = true
					if p.deps.StakeRoots != nil {
						if newRoot, rootErr := p.deps.StakeTree.Root(); rootErr == nil {
							p.deps.StakeRoots.Record(newRoot)
						}
					}
				}
			}
		} else if record.Passed && record.MintAmount > 0 {
			// Real spec-17.4 direct-mint execution: MintOutCommit's real Groth16
			// proof was already verified once, at Stage 4, the moment
			// this proposal's first vote bound it (see that case's own
			// doc) — nothing here re-proves soundness, it only makes a
			// governance-authorized amount of new SFG real by inserting
			// the same commitment into the same canonical tree a
			// Transfer's own output commitments live in, so it becomes a
			// genuinely spendable note. ZKTree == nil (never true on a
			// real node — see Deps.ZKTree's own doc) leaves MintApplied
			// false, the durable signal this mint never actually
			// executed, mirroring Applied's identical treatment above.
			if p.deps.ZKTree != nil {
				if _, err := p.deps.ZKTree.Insert(zk.FieldElementFromBytes32(record.MintOutCommit)); err == nil {
					p.deps.StateTree.Append(record.MintOutCommit)
					record.MintApplied = true
					if p.deps.ZKRoots != nil {
						if newRoot, rootErr := p.deps.ZKTree.Root(); rootErr == nil {
							p.deps.ZKRoots.Record(newRoot)
						}
					}
					if p.deps.Vault != nil {
						p.deps.Vault.CollectFee(decimal.FromInt(int64(types.MintFeeAmount(record.MintAmount))))
					}
				}
			}
		}

		if record.Passed && !record.SlashTargetNFT.IsZero() {
			// Real spec-10.3 slash execution: the target's real existence
			// was already checked once, at Stage 4, the moment this
			// proposal's first vote bound it (see that case's own doc) —
			// this only makes a governance-authorized slash real. A
			// target that no longer exists by tally time (e.g. a second,
			// independent slash proposal against the same NFT that
			// happened to pass and tally first) leaves SlashApplied
			// false, the same durable "never actually executed" signal
			// MintApplied gives for the mint paths above, rather than a
			// hard tally failure — one proposal's outcome must never
			// block tallying every other due proposal in the same call.
			if nftRec, found, err := p.deps.Store.GetNFT(record.SlashTargetNFT); err == nil && found {
				action := nft.SlashFreeze
				if record.SlashBurn {
					action = nft.SlashBurn
				}
				nft.ApplySlash(&nftRec, action)
				if record.SlashBurn {
					if err := p.deps.Store.DeleteNFT(nftRec); err == nil {
						record.SlashApplied = true
					}
				} else {
					if err := p.deps.Store.PutNFT(nftRec); err == nil {
						record.SlashApplied = true
					}
				}
			}
		}

		if err := p.deps.Store.PutProposal(record); err != nil {
			return nil, fmt.Errorf("persist tally for proposal %s: %w", record.ProposalID, err)
		}
		tallied = append(tallied, record)
	}
	return tallied, nil
}
