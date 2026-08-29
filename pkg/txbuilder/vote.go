package txbuilder

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/staking"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// voteNonceDomain separates the deterministic vote-nonce derivation below
// from any other use of SumHash(sk, ...) elsewhere, so the same private
// key can never accidentally produce the same output for two different
// purposes.
var voteNonceDomain = []byte("shadowforge-txbuilder-vote-nonce-v1")

// voteNonce deterministically derives this ballot's sealed nonce for
// proposalID from the voter's real eligibility nullifier, rather than
// drawing a fresh random one and requiring the caller to remember it
// until reveal time. This is a real, deliberate design choice: a wallet
// that stores nothing between casting a vote and revealing it — no local
// "pending votes" database to lose or restore from backup — can still
// reveal correctly, because VoteReveal recomputes the identical nonce
// from the same (voterNullifier, proposal ID) pair.
//
// This intentionally does NOT derive from b.sk (unlike, e.g., NFTMint's
// nonce elsewhere in this package): Vote and VoteReveal are built by two
// separate Builder values wrapping two different, unlinked keys (see
// Vote's own doc on why b should hold a fresh throwaway key per call,
// distinct from the identity that minted the NFT) — deriving from b.sk
// would make VoteReveal recompute a DIFFERENT nonce than the Vote it's
// meant to open, since it's a different b. voterNullifier is exactly the
// one value the real design guarantees is identical across both calls
// (deterministic in VoterSK+proposalID — see pkg/govwallet.Wallet.
// BuildEligibilityProof), so it is what ties nonce (and therefore
// Commitment) to one specific voter/proposal pair without touching a
// signing key at all.
func voteNonce(proposalID types.ID, voterNullifier types.Hash) types.Hash {
	return types.SumHash(voteNonceDomain, []byte(proposalID), voterNullifier[:])
}

// Vote casts a real sealed ballot for proposalID: approve stays hidden
// inside Commitment until a later VoteReveal opens it, matching exactly
// what pkg/tx's pipeline (Stage 4, TxVote case) checks — the same real
// commit-reveal scheme cmd/walletsim already exercises for one kind,
// extended here into a reusable, tested builder.
//
// eligibility is a real, pre-built anonymous ZK proof that this caller
// holds a minted NFT (pkg/govwallet.Wallet.BuildEligibilityProof) —
// Builder itself never touches the network or a Merkle tree (see the
// package doc), so unlike Identity() it cannot build one on its own. Its
// Nullifier, not b.Identity(), is what Commitment binds and what the
// pipeline dedups ballots by (types.VoteEligibilityProof's own doc): this
// is what makes a ballot anonymous rather than tied to b's own signing
// key. For that anonymity to mean anything, b should hold a fresh key
// generated just for this vote, unlinked from whatever identity minted
// the NFT eligibility proves — pkg/govwallet.Wallet.BuildEligibilityProof
// derives its proof from a separate, deterministic secret of its own,
// independent of whatever key signs the transaction this method returns.
//
// paramKey/newValue optionally bind this proposal to a real
// governance.Params field change (see types.VotePublicInputs' own doc):
// pass "" for both for a plain up/down vote. They only matter on the
// first Vote to ever reference proposalID — a later voter's own values
// are ignored by the pipeline, not by this builder, so passing something
// here doesn't guarantee it takes effect if someone else voted first.
func (b *Builder) Vote(proposalID types.ID, approve bool, paramKey, newValue string, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: proposalID,
			Commitment: commitment,
			ParamKey:   paramKey,
			NewValue:   newValue,
		},
		VoteEligibility: &eligibility,
		// Distinct from VoteReveal's nullifier for the same proposal (see
		// that function) — reusing one would collide the pair's TxIDs and
		// have the mempool's TxID-based dedup silently drop the second.
		// This is the top-level spec-4.1 TxID nullifier (b's own signing
		// key, purely for TxID uniqueness), not eligibility.Nullifier
		// (the real anonymous per-proposal ballot nullifier) — the two
		// serve entirely different purposes and must never be confused.
		Nullifier: types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("commit")),
	}
	return b.finalize(t)
}

// ProposeMint is Vote's real spec-17.4 epoch-mint counterpart: it casts
// a real sealed ballot for proposalID exactly like Vote does, and also
// binds it to a real, Groth16-proven request to mint amount SFG on the
// "direct with 10 percent fee" proposer path (see types.
// VotePublicInputs.MintAmount's own doc for why this build implements
// only that path, not the spec's staked-yield alternative). Like
// ParamKey/NewValue, the mint claim only matters on the first Vote/
// ProposeMint call to ever reference proposalID; the pipeline verifies
// it once, right there, and ignores a later caller's own claim.
//
// sys must be the exact same real, shared zk.MintSystem every validator
// verifying this proposal loads (mirroring every other real proof this
// package or pkg/shieldedwallet builds — see 'wallet mint-zk-setup').
// amount must be > 0. Returns the real zk.NoteSecret opening the
// resulting output note (MintOutCommit) alongside the transaction: this
// package never persists anything (see the package doc), so the caller
// alone is responsible for remembering it — it is the only way to ever
// spend the minted value later, exactly like remembering a Transfer's
// own change note.
func (b *Builder) ProposeMint(proposalID types.ID, approve bool, amount uint64, sys *zk.MintSystem, eligibility types.VoteEligibilityProof) (types.ShieldedTx, zk.NoteSecret, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	if amount == 0 {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: mint amount must be greater than 0")
	}
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: generate mint note owner key: %w", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: generate mint note rho: %w", err)
	}
	secret := zk.NoteSecret{Value: types.MintNetAmount(amount), OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: prove mint: %w", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: serialize mint proof: %w", err)
	}

	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:    proposalID,
			Commitment:    commitment,
			MintAmount:    amount,
			MintOutCommit: types.Hash(zk.ToBytes32(secret.Commitment())),
			MintProof:     proofBytes,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("mint-commit")),
	}
	finalized, err := b.finalize(t)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, err
	}
	return finalized, secret, nil
}

// ProposeMintStaked is ProposeMint's spec-17.4 "staked 2 percent yield"
// counterpart: it casts a real sealed ballot for proposalID exactly like
// ProposeMint does, and binds it to a real, Groth16-proven request to
// lock amount SFG as a staked position rather than mint it directly (see
// types.VotePublicInputs.MintStaked's own doc, and pkg/staking's own doc
// for the real yield formula this build implements for the spec's
// otherwise-underspecified "2 percent yield").
//
// currentEpoch must be this node's own real current epoch at the moment
// this proposal is first created — pkg/tx's Stage 4 checks the resulting
// proof was built for exactly that epoch (a proposer cannot pick an
// earlier one to front-run extra yield); the caller is responsible for
// supplying it correctly (mirroring Deps.Epoch elsewhere in this
// codebase).
//
// sys must be the exact same real, shared zk.StakeSystem every validator
// verifying this proposal loads (mirroring ProposeMint's own sys
// parameter). amount must be > 0. Returns the real zk.StakeSecret
// opening the resulting locked position: this package never persists
// anything (see the package doc), so the caller alone is responsible for
// remembering it — it is the only way to ever redeem the staked value
// later, via Unstake.
func (b *Builder) ProposeMintStaked(proposalID types.ID, approve bool, amount uint64, currentEpoch types.EpochNumber, sys *zk.StakeSystem, eligibility types.VoteEligibilityProof) (types.ShieldedTx, zk.StakeSecret, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	if amount == 0 {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: mint amount must be greater than 0")
	}
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: generate stake position owner key: %w", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: generate stake position rho: %w", err)
	}
	secret := zk.StakeSecret{Principal: amount, StartEpoch: uint64(currentEpoch), OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: prove stake: %w", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		return types.ShieldedTx{}, zk.StakeSecret{}, fmt.Errorf("txbuilder: serialize stake proof: %w", err)
	}

	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:          proposalID,
			Commitment:          commitment,
			MintAmount:          amount,
			MintStaked:          true,
			StakePositionCommit: types.Hash(zk.ToBytes32(secret.Commitment())),
			StakeProof:          proofBytes,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("stake-mint-commit")),
	}
	finalized, err := b.finalize(t)
	if err != nil {
		return types.ShieldedTx{}, zk.StakeSecret{}, err
	}
	return finalized, secret, nil
}

// Unstake redeems position — a real staked position ProposeMintStaked
// created and this proposal already passed and was tallied — for a
// fresh, ordinary spendable note carrying the position's principal plus
// its real accrued yield.
//
// merkleProof is the position's own real membership witness in the
// canonical stake-commitment tree (this package never touches a network
// or a tree — see the package doc — so unlike Vote's eligibility
// parameter, the caller must supply this directly, e.g. via
// pkg/stakewallet's real sync). merkleRoot must be a root pkg/tx's
// pipeline still recognizes (its real StakeRoots history tolerates
// staleness the same way Transfer's own root check does — see
// zk.RootHistory's own doc — but an arbitrarily old root may eventually
// age out).
//
// currentEpoch must be this node's own real current epoch: unlike
// ProposeMintStaked's own currentEpoch parameter (which becomes
// cryptographically pinned into the position's commitment), this one
// only determines the real yield this call computes and proves — pkg/tx's
// Stage 1 independently recomputes the identical formula against its own
// view of the current epoch and rejects the transaction if they disagree
// (see types.UnstakePublicInputs' own doc), so submitting stale is safe
// to attempt but not guaranteed to land; the caller should use the most
// recent epoch it knows of.
//
// sys must be the exact same real, shared zk.UnstakeSystem every
// validator verifying this transaction loads. Returns the real
// zk.NoteSecret opening the resulting proceeds note, for the identical
// reason ProposeMint/ProposeMintStaked return theirs.
func (b *Builder) Unstake(position zk.StakeSecret, merkleProof zk.Proof, merkleRoot zk.FieldElement, currentEpoch types.EpochNumber, sys *zk.UnstakeSystem) (types.ShieldedTx, zk.NoteSecret, error) {
	finalAmount := staking.FinalAmount(position.Principal, types.EpochNumber(position.StartEpoch), currentEpoch)

	outOwnerSK, err := zk.NewSpendKey()
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: generate unstake proceeds owner key: %w", err)
	}
	outRho, err := zk.NewRho()
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: generate unstake proceeds rho: %w", err)
	}
	out := zk.NoteSecret{Value: finalAmount, OwnerSK: outOwnerSK, Rho: outRho}

	in := zk.UnstakeInput{MerkleRoot: merkleRoot, Position: position, Proof: merkleProof, Out: out}
	proof, err := sys.Prove(in)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: prove unstake: %w", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, fmt.Errorf("txbuilder: serialize unstake proof: %w", err)
	}

	t := types.ShieldedTx{
		Kind:        types.TxUnstake,
		Proof:       proofBytes,
		Nullifier:   types.Hash(zk.ToBytes32(in.Nullifier())),
		Commitments: []types.Hash{types.Hash(zk.ToBytes32(out.Commitment()))},
		UnstakePublicInputs: &types.UnstakePublicInputs{
			MerkleRoot:  types.Hash(zk.ToBytes32(merkleRoot)),
			Principal:   position.Principal,
			StartEpoch:  types.EpochNumber(position.StartEpoch),
			FinalAmount: finalAmount,
		},
	}
	finalized, err := b.finalize(t)
	if err != nil {
		return types.ShieldedTx{}, zk.NoteSecret{}, err
	}
	return finalized, out, nil
}

// ProposeSlash casts a real sealed ballot for proposalID exactly like
// Vote does, and binds it to a real spec-10.3 slash request against
// target (see types.VotePublicInputs.SlashTargetNFT's own doc). Unlike
// ProposeMint/ProposeMintStaked, this needs no Groth16 proof of its
// own: pkg/tx's Stage 4 checks target really exists at the moment this
// binds, and the actual slash — freeze (burn == false) or a real record
// deletion (burn == true) — runs once, at tally, if the proposal
// passes. Like ParamKey/NewValue, the claim only matters on the first
// Vote/ProposeSlash call to ever reference proposalID; the pipeline
// verifies it once, right there, and ignores a later caller's own
// claim.
func (b *Builder) ProposeSlash(proposalID types.ID, approve bool, target types.NFTID, burn bool, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	if target.IsZero() {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: slash target must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:     proposalID,
			Commitment:     commitment,
			SlashTargetNFT: target,
			SlashBurn:      burn,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("slash-commit")),
	}
	return b.finalize(t)
}

// ProposeUnlockTransfer casts a real sealed ballot for proposalID exactly
// like ProposeSlash does, and binds it to a real spec-10.1 "unlock a
// transfer trait" request against target (see types.VotePublicInputs.
// UnlockTransferTarget's own doc). Like ProposeSlash, this needs no
// Groth16 proof of its own — pkg/tx's Stage 4 checks target really
// exists at the moment this binds, and the actual unlock runs once, at
// tally, if the proposal passes.
func (b *Builder) ProposeUnlockTransfer(proposalID types.ID, approve bool, target types.NFTID, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	if target.IsZero() {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: unlock-transfer target must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:           proposalID,
			Commitment:           commitment,
			UnlockTransferTarget: target,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("unlock-transfer-commit")),
	}
	return b.finalize(t)
}

// ProposeAuthorizeAsset casts a real sealed ballot for proposalID exactly
// like ProposeSlash/ProposeUnlockTransfer do, and binds it to a real
// spec-11/19.3 "authorize a new Bank asset" request against asset (see
// types.VotePublicInputs.ContainerAssetTarget's own doc). Like
// ProposeSlash/ProposeUnlockTransfer, this needs no Groth16 proof of its
// own — pkg/tx's Stage 4 checks asset isn't the native SFG and isn't
// already authorized at the moment this binds, and the actual
// authorization (state.Store.PutAuthorizedAsset) runs once, at tally, if
// the proposal passes.
func (b *Builder) ProposeAuthorizeAsset(proposalID types.ID, approve bool, asset types.AssetID, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	if asset == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: container-asset target must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:           proposalID,
			Commitment:           commitment,
			ContainerAssetTarget: asset,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("container-asset-commit")),
	}
	return b.finalize(t)
}

// VoteReveal opens the sealed ballot Vote(proposalID, approve, ...)
// earlier committed, by recomputing the same deterministic nonce and
// handing back the (approve, nonce) pair the pipeline checks against the
// stored commitment. approve must be the exact same value passed to the
// matching Vote call — VoteReveal has no way to recover it on its own
// (that's the whole point of a sealed ballot), so the caller (a wallet's
// own UI, or its own record of what it voted) is responsible for
// supplying it correctly; passing the wrong value produces a
// well-formed but honestly-rejected reveal, exactly as if a stranger
// tried to open someone else's ballot.
//
// eligibility must resolve to the exact same Nullifier as the matching
// Vote call's own eligibility proof — pkg/govwallet.Wallet.
// BuildEligibilityProof(sys, proposalID) reproduces it deterministically,
// so calling it again here (even with a freshly built proof) still ties
// this reveal to the same earlier commitment; a proof for a different
// proposalID or a different underlying VoterSK cannot open it.
func (b *Builder) VoteReveal(proposalID types.ID, approve bool, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)

	t := types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: proposalID,
			Approve:    approve,
			Nonce:      nonce,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("reveal")),
	}
	return b.finalize(t)
}
