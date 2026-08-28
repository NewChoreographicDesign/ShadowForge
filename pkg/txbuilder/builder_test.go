package txbuilder_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// realEligibilitySystem lazily runs one real Groth16 setup for
// zk.EligibilityCircuit and shares it across every test in this file —
// mirroring pkg/tx/pipeline_test.go's own zkOnce pattern for
// TransferCircuit: a real setup is expensive enough that every test
// building its own would slow the suite for no real benefit, and every
// test needs the exact same proving/verifying keys to interoperate
// anyway.
var (
	eligOnce sync.Once
	eligSys  *zk.EligibilitySystem
	eligErr  error
)

func realEligibilitySystem(t *testing.T) *zk.EligibilitySystem {
	t.Helper()
	eligOnce.Do(func() { eligSys, eligErr = zk.SetupEligibility() })
	if eligErr != nil {
		t.Fatalf("eligibility zk setup: %v", eligErr)
	}
	return eligSys
}

func openStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("txbuilder-test-key-32-byte-pad!!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newRealPipeline builds a genuine tx.Pipeline — the exact same
// validation code a live node runs — so every test in this file proves
// txbuilder's output against real acceptance logic, not a mock of it.
// Every pipeline gets a real, shared EligibilityZK system and a fresh
// EligibilityTree/EligibilityRoots pair — TxVote/TxVoteReveal's real
// anonymous eligibility check is unconditional (fail-closed), so any
// test exercising them needs these wired regardless of extra.
func newRealPipeline(t *testing.T, extra tx.Deps) (*tx.Pipeline, tx.Deps) {
	t.Helper()
	eligTree := zk.NewTree()
	initialRoot, err := eligTree.Root()
	if err != nil {
		t.Fatalf("fresh eligibility tree root: %v", err)
	}
	deps := tx.Deps{
		Store:            openStore(t),
		StateTree:        state.NewMerkleTree(),
		Oracle:           extra.Oracle,
		OracleTolerance:  extra.OracleTolerance,
		Epoch:            extra.Epoch,
		EligibilityZK:    realEligibilitySystem(t),
		EligibilityTree:  eligTree,
		EligibilityRoots: zk.NewRootHistory(initialRoot),
	}
	return tx.NewPipeline(deps), deps
}

func newIdentity(t *testing.T) *txbuilder.Builder {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return txbuilder.New(pk, sk)
}

// voterIdentity is a real identity that has both minted a ValidatorNFT
// (deps.Store) and registered its real anonymous VoterCommitment in
// deps.EligibilityTree, so it can build a genuine VoteEligibilityProof
// for any proposal via eligibilityFor. It embeds *txbuilder.Builder so
// every ordinary Builder method (Vote, VoteReveal, Identity, ...) is
// still available directly.
type voterIdentity struct {
	*txbuilder.Builder
	voterSK   zk.FieldElement
	treeIndex int
	tree      *zk.Tree
	sys       *zk.EligibilitySystem
}

// newVoterIdentity is newIdentity plus a real, minted ValidatorNFT seeded
// directly into deps.Store and a real VoterCommitment leaf inserted into
// deps.EligibilityTree — real voter eligibility (pkg/tx's
// requireEligibleVoterZK) is unconditional, so any test proving TxVote/
// TxVoteReveal acceptance through the real pipeline needs this instead
// of newIdentity.
func newVoterIdentity(t *testing.T, deps tx.Deps) *voterIdentity {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	owner := types.AddressFromPubkey(pk)
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(owner[:])), Owner: owner}); err != nil {
		t.Fatalf("seed voter nft: %v", err)
	}
	voterSK := zk.DeriveVoterSK(sk)
	idx, err := deps.EligibilityTree.Insert(zk.VoterCommitment(voterSK))
	if err != nil {
		t.Fatalf("insert voter commitment: %v", err)
	}
	newRoot, err := deps.EligibilityTree.Root()
	if err != nil {
		t.Fatalf("eligibility tree root: %v", err)
	}
	deps.EligibilityRoots.Record(newRoot)
	return &voterIdentity{
		Builder:   txbuilder.New(pk, sk),
		voterSK:   voterSK,
		treeIndex: idx,
		tree:      deps.EligibilityTree,
		sys:       deps.EligibilityZK,
	}
}

// eligibilityFor builds a real, fresh zk.EligibilitySystem proof that v
// holds a real, minted NFT eligible to vote on proposalID, without
// revealing which leaf it is — exactly what a real wallet
// (pkg/govwallet.Wallet.BuildEligibilityProof) would produce, built here
// directly against the same in-process tree/system rather than over a
// live network, since these are txbuilder-level unit tests.
func (v *voterIdentity) eligibilityFor(t *testing.T, proposalID types.ID) types.VoteEligibilityProof {
	t.Helper()
	proof, err := v.tree.Prove(v.treeIndex)
	if err != nil {
		t.Fatalf("merkle proof: %v", err)
	}
	scope := zk.FieldElementFromBytes32(types.VoteEligibilityScope(proposalID))
	in := zk.EligibilityInput{MerkleRoot: proof.Root, ProposalScope: scope, VoterSK: v.voterSK, Proof: proof}
	zproof, err := v.sys.Prove(in)
	if err != nil {
		t.Fatalf("prove eligibility: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(zproof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	return types.VoteEligibilityProof{
		Proof:      proofBytes,
		MerkleRoot: types.Hash(zk.ToBytes32(proof.Root)),
		Nullifier:  types.Hash(zk.ToBytes32(in.Nullifier())),
	}
}

func runOne(p *tx.Pipeline, txn types.ShieldedTx) error {
	results := p.ProcessBatch([]tx.Entry{{Tx: txn}})
	return results[0].Error
}

// assertRealSignature proves the constructed transaction's Sig genuinely
// verifies against SignerPubKey with pkg/crypto's real Dilithium check —
// independent of whether the pipeline happens to accept it, since a
// pipeline rejection for an unrelated reason should never mask a broken
// signature.
func assertRealSignature(t *testing.T, txn types.ShieldedTx) {
	t.Helper()
	if len(txn.Sig) == 0 || len(txn.SignerPubKey) == 0 {
		t.Fatalf("expected a real signature and signer public key, got Sig=%d bytes SignerPubKey=%d bytes", len(txn.Sig), len(txn.SignerPubKey))
	}
	ok, err := crypto.DilithiumVerify(crypto.DilithiumPublicKey(txn.SignerPubKey), txn.TxID[:], crypto.DilithiumSignature(txn.Sig))
	if err != nil || !ok {
		t.Fatalf("expected a genuinely verifiable signature: ok=%v err=%v", ok, err)
	}
	wantTxID := types.ComputeTxID(txn.Proof, txn.Commitments, txn.Nullifier)
	if txn.TxID != wantTxID {
		t.Fatalf("TxID does not match Hash(proof||commitments||nullifier)")
	}
}

func TestBuilderIdentityMatchesConsensusConvention(t *testing.T) {
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b := txbuilder.New(pk, sk)
	want := types.NFTID(types.SumHash(pk))
	if b.Identity() != want {
		t.Fatalf("Identity() does not match NFTID(SumHash(publicKey)), the convention pkg/validator and cmd/walletsim use")
	}
}

// --- Vote / VoteReveal ---

func TestVoteAcceptedByRealPipelineAndRecordsCommitment(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "prop-1")

	votetx, err := b.Vote("prop-1", true, "", "", elig)
	if err != nil {
		t.Fatalf("build vote: %v", err)
	}
	assertRealSignature(t, votetx)

	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed Vote: %v", err)
	}

	record, found, err := deps.Store.GetProposal("prop-1")
	if err != nil || !found {
		t.Fatalf("expected a real proposal record to exist: found=%v err=%v", found, err)
	}
	if _, voted := record.Commitments[elig.Nullifier]; !voted {
		t.Fatalf("expected this identity's commitment to be recorded")
	}
}

func TestVoteWithParamKeyBindsFirstVoteOnly(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	first := newVoterIdentity(t, deps)
	second := newVoterIdentity(t, deps)

	firstVote, err := first.Vote("prop-param", true, "DepositATRMultiple", "4", first.eligibilityFor(t, "prop-param"))
	if err != nil {
		t.Fatalf("build first vote: %v", err)
	}
	if err := runOne(p, firstVote); err != nil {
		t.Fatalf("first vote: %v", err)
	}

	secondVote, err := second.Vote("prop-param", false, "WithdrawATRMultiple", "9", second.eligibilityFor(t, "prop-param"))
	if err != nil {
		t.Fatalf("build second vote: %v", err)
	}
	if err := runOne(p, secondVote); err != nil {
		t.Fatalf("second vote: %v", err)
	}

	record, found, err := deps.Store.GetProposal("prop-param")
	if err != nil || !found {
		t.Fatalf("expected a proposal record: found=%v err=%v", found, err)
	}
	if record.ParamKey != "DepositATRMultiple" || record.NewValue != "4" {
		t.Fatalf("expected the FIRST voter's ParamKey/NewValue to stick, got %q/%q", record.ParamKey, record.NewValue)
	}
}

func TestSecondVoteFromSameIdentityRejectedByRealPipeline(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "prop-dup")

	first, err := b.Vote("prop-dup", true, "", "", elig)
	if err != nil {
		t.Fatalf("build first vote: %v", err)
	}
	if err := runOne(p, first); err != nil {
		t.Fatalf("first vote should be accepted: %v", err)
	}

	// The deterministic nonce means calling Vote again with the same
	// (proposal, approve, eligibility) reproduces the identical
	// transaction — proving the idempotency property this package's doc
	// promises, while also exercising the pipeline's real
	// one-NFT-one-vote (nullifier dedup) rejection.
	second, err := b.Vote("prop-dup", true, "", "", elig)
	if err != nil {
		t.Fatalf("build second vote: %v", err)
	}
	if second.TxID != first.TxID {
		t.Fatalf("expected a repeated identical Vote call to be idempotent (same TxID)")
	}
	if err := runOne(p, second); err == nil {
		t.Fatalf("expected the real pipeline to reject a second ballot from the same identity")
	}
}

func TestVoteRevealRoundTripAcceptedByRealPipeline(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "prop-reveal")

	votetx, err := b.Vote("prop-reveal", true, "", "", elig)
	if err != nil {
		t.Fatalf("build vote: %v", err)
	}
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("vote: %v", err)
	}

	revealtx, err := b.VoteReveal("prop-reveal", true, b.eligibilityFor(t, "prop-reveal"))
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	assertRealSignature(t, revealtx)
	if err := runOne(p, revealtx); err != nil {
		t.Fatalf("expected the real pipeline to accept a matching reveal: %v", err)
	}

	record, found, err := deps.Store.GetProposal("prop-reveal")
	if err != nil || !found {
		t.Fatalf("expected a proposal record: found=%v err=%v", found, err)
	}
	if approve, revealed := record.Reveals[elig.Nullifier]; !revealed || !approve {
		t.Fatalf("expected a real, correct reveal recorded: revealed=%v approve=%v", revealed, approve)
	}
}

func TestVoteRevealRejectsWrongApproveValue(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)

	votetx, err := b.Vote("prop-mismatch", true, "", "", b.eligibilityFor(t, "prop-mismatch"))
	if err != nil {
		t.Fatalf("build vote: %v", err)
	}
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("vote: %v", err)
	}

	// Reveal claims the opposite of what was actually committed.
	wrongReveal, err := b.VoteReveal("prop-mismatch", false, b.eligibilityFor(t, "prop-mismatch"))
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	if err := runOne(p, wrongReveal); err == nil {
		t.Fatalf("expected the real pipeline to reject a reveal that doesn't match the earlier commitment")
	}
}

func TestVoteRevealWithoutPriorVoteRejectedByRealPipeline(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)

	revealtx, err := b.VoteReveal("prop-never-voted", true, b.eligibilityFor(t, "prop-never-voted"))
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	if err := runOne(p, revealtx); err == nil {
		t.Fatalf("expected the real pipeline to reject a reveal with no matching vote")
	}
}

func TestVoteRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.Vote("", true, "", "", types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestVoteRevealRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.VoteReveal("", true, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

// --- Bank ---

func staticQuorum(t *testing.T, priceUSD, atrUSD string) *oracle.Quorum {
	t.Helper()
	return oracle.NewQuorum(decimal.MustFromString("0.02"), oracle.StaticSource{
		Value: oracle.Quote{PriceUSD: decimal.MustFromString(priceUSD), ATRUSD: decimal.MustFromString(atrUSD)},
	})
}

func TestBankDepositAcceptedByRealPipelineWithMatchingOracle(t *testing.T) {
	quorum := staticQuorum(t, "60000", "1500")
	p, _ := newRealPipeline(t, tx.Deps{Oracle: quorum})
	b := newIdentity(t)

	deposit, err := b.BankDeposit(quorum, "BTC")
	if err != nil {
		t.Fatalf("build deposit: %v", err)
	}
	assertRealSignature(t, deposit)
	if err := runOne(p, deposit); err != nil {
		t.Fatalf("expected the real pipeline to accept a deposit whose claim matches its own oracle: %v", err)
	}
}

func TestBankWithdrawAcceptedByRealPipelineWithMatchingOracle(t *testing.T) {
	quorum := staticQuorum(t, "60000", "1500")
	p, _ := newRealPipeline(t, tx.Deps{Oracle: quorum})
	b := newIdentity(t)

	withdraw, err := b.BankWithdraw(quorum, "BTC")
	if err != nil {
		t.Fatalf("build withdraw: %v", err)
	}
	assertRealSignature(t, withdraw)
	if err := runOne(p, withdraw); err != nil {
		t.Fatalf("expected the real pipeline to accept a withdrawal whose claim matches its own oracle: %v", err)
	}
}

// TestBankDepositRejectedWhenPipelineOracleDisagrees proves the built
// transaction's claimed price is really checked against a node's own
// oracle reading, not merely internally self-consistent: the transaction
// is built against one quorum reading and then checked by a pipeline
// wired to a different, disagreeing one.
func TestBankDepositRejectedWhenPipelineOracleDisagrees(t *testing.T) {
	buildQuorum := staticQuorum(t, "60000", "1500")
	nodeQuorum := staticQuorum(t, "90000", "1500") // 50% higher price than what the tx will claim
	p, _ := newRealPipeline(t, tx.Deps{Oracle: nodeQuorum})
	b := newIdentity(t)

	deposit, err := b.BankDeposit(buildQuorum, "BTC")
	if err != nil {
		t.Fatalf("build deposit: %v", err)
	}
	if err := runOne(p, deposit); err == nil {
		t.Fatalf("expected the real pipeline to reject a deposit whose claimed price diverges from its own oracle")
	}
}

func TestBankDepositFromQuoteBufferMatchesRealMultiple(t *testing.T) {
	b := newIdentity(t)
	quote := oracle.Quote{PriceUSD: decimal.MustFromString("100"), ATRUSD: decimal.MustFromString("10")}
	deposit, err := b.BankDepositFromQuote("BTC", quote)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// bank.DepositATRMultiple is 2.5 — buffer must be exactly 25.
	if deposit.BankPublicInputs.BufferUSD.String() != "25" {
		t.Fatalf("expected buffer 25 (2.5 * 10), got %s", deposit.BankPublicInputs.BufferUSD)
	}
}

func TestBankWithdrawFromQuoteBufferMatchesRealMultiple(t *testing.T) {
	b := newIdentity(t)
	quote := oracle.Quote{PriceUSD: decimal.MustFromString("100"), ATRUSD: decimal.MustFromString("10")}
	withdraw, err := b.BankWithdrawFromQuote("BTC", quote)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// bank.WithdrawATRMultiple is 1.5 — buffer must be exactly 15.
	if withdraw.BankPublicInputs.BufferUSD.String() != "15" {
		t.Fatalf("expected buffer 15 (1.5 * 10), got %s", withdraw.BankPublicInputs.BufferUSD)
	}
}

// --- Mint ---

func TestMintAcceptedByRealPipeline(t *testing.T) {
	p, _ := newRealPipeline(t, tx.Deps{})
	b := newIdentity(t)

	mint, err := b.Mint()
	if err != nil {
		t.Fatalf("build mint: %v", err)
	}
	assertRealSignature(t, mint)
	if err := runOne(p, mint); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed Mint: %v", err)
	}
}

func TestTwoMintsFromSameIdentityHaveDistinctTxIDs(t *testing.T) {
	b := newIdentity(t)
	a, err := b.Mint()
	if err != nil {
		t.Fatalf("mint a: %v", err)
	}
	c, err := b.Mint()
	if err != nil {
		t.Fatalf("mint b: %v", err)
	}
	if a.TxID == c.TxID {
		t.Fatalf("expected two separate Mint calls to produce distinct TxIDs (random nullifier)")
	}
}

// --- NFTMint ---

func genAttestor(t *testing.T) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey) {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate attestor key: %v", err)
	}
	return pk, sk
}

// ownerOf derives b's real types.Address the same way pkg/tx's pipeline
// does when checking a TxNFTMint (types.AddressFromPubkey), so a test's
// locally-signed PoHAttestation binds to exactly the owner the pipeline
// will look up.
func ownerOf(b *txbuilder.Builder) types.Address {
	return types.AddressFromPubkey(b.PublicKey())
}

func TestNFTMintAcceptedByRealPipelineWithTrustedAttestation(t *testing.T) {
	attestorPK, attestorSK := genAttestor(t)
	_, deps := newRealPipeline(t, tx.Deps{})
	deps.TrustedPoHAttestors = []crypto.DilithiumPublicKey{attestorPK}
	p := tx.NewPipeline(deps)

	b := newIdentity(t)
	now := time.Now()
	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, ownerOf(b), 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}

	mintTx, err := b.NFTMint(1, att.IssuedAtMs, att.Attestor, att.Sig)
	if err != nil {
		t.Fatalf("build nft mint: %v", err)
	}
	assertRealSignature(t, mintTx)
	if err := runOne(p, mintTx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed, trusted-attestor-signed mint: %v", err)
	}

	minted, found, err := deps.Store.GetNFTByOwner(ownerOf(b))
	if err != nil || !found {
		t.Fatalf("expected a real minted NFT: found=%v err=%v", found, err)
	}
	if minted.Slashed {
		t.Fatalf("expected a freshly minted NFT to not be slashed")
	}
}

func TestNFTMintRejectedByUntrustedAttestor(t *testing.T) {
	_, untrustedSK := genAttestor(t)
	untrustedPK, _ := genAttestor(t)
	trustedPK, _ := genAttestor(t)
	_, deps := newRealPipeline(t, tx.Deps{})
	deps.TrustedPoHAttestors = []crypto.DilithiumPublicKey{trustedPK}
	p := tx.NewPipeline(deps)

	b := newIdentity(t)
	now := time.Now()
	att, err := nft.SignPoHAttestation(untrustedPK, untrustedSK, ownerOf(b), 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	mintTx, err := b.NFTMint(1, att.IssuedAtMs, att.Attestor, att.Sig)
	if err != nil {
		t.Fatalf("build nft mint: %v", err)
	}
	if err := runOne(p, mintTx); err == nil {
		t.Fatalf("expected the real pipeline to reject a mint attested by an untrusted attestor")
	}
}

func TestNFTMintRejectedWhenNoAttestorTrusted(t *testing.T) {
	attestorPK, attestorSK := genAttestor(t)
	p, _ := newRealPipeline(t, tx.Deps{}) // deps.TrustedPoHAttestors left nil: fail closed
	b := newIdentity(t)
	now := time.Now()
	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, ownerOf(b), 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	mintTx, err := b.NFTMint(1, att.IssuedAtMs, att.Attestor, att.Sig)
	if err != nil {
		t.Fatalf("build nft mint: %v", err)
	}
	if err := runOne(p, mintTx); err == nil {
		t.Fatalf("expected a mint attempt to be rejected when no attestor is trusted (fail closed)")
	}
}

func TestNFTMintRejectsSecondMintForSameWallet(t *testing.T) {
	attestorPK, attestorSK := genAttestor(t)
	_, deps := newRealPipeline(t, tx.Deps{})
	deps.TrustedPoHAttestors = []crypto.DilithiumPublicKey{attestorPK}
	p := tx.NewPipeline(deps)

	b := newIdentity(t)
	now := time.Now()
	att1, err := nft.SignPoHAttestation(attestorPK, attestorSK, ownerOf(b), 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation 1: %v", err)
	}
	first, err := b.NFTMint(1, att1.IssuedAtMs, att1.Attestor, att1.Sig)
	if err != nil {
		t.Fatalf("build first mint: %v", err)
	}
	if err := runOne(p, first); err != nil {
		t.Fatalf("expected the first mint to succeed: %v", err)
	}

	att2, err := nft.SignPoHAttestation(attestorPK, attestorSK, ownerOf(b), 2, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation 2: %v", err)
	}
	second, err := b.NFTMint(2, att2.IssuedAtMs, att2.Attestor, att2.Sig)
	if err != nil {
		t.Fatalf("build second mint: %v", err)
	}
	if err := runOne(p, second); err == nil {
		t.Fatalf("expected a second mint attempt from the same wallet to be rejected (one per wallet)")
	}
}

func TestNFTMintRejectsExpiredAttestation(t *testing.T) {
	attestorPK, attestorSK := genAttestor(t)
	_, deps := newRealPipeline(t, tx.Deps{})
	deps.TrustedPoHAttestors = []crypto.DilithiumPublicKey{attestorPK}
	p := tx.NewPipeline(deps)

	b := newIdentity(t)
	stale := time.Now().Add(-2 * nft.PoHAttestationTTL)
	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, ownerOf(b), 1, stale.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	mintTx, err := b.NFTMint(1, att.IssuedAtMs, att.Attestor, att.Sig)
	if err != nil {
		t.Fatalf("build nft mint: %v", err)
	}
	if err := runOne(p, mintTx); err == nil {
		t.Fatalf("expected the real pipeline to reject an expired attestation")
	}
}

// --- NFTTrait ---

func TestNFTTraitAcceptedByRealPipelineAndUpdatesRecord(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newIdentity(t)

	target := types.NFTID{0x42}
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: target, Owner: types.Address{0x01}}); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	trait, err := b.NFTTrait(target, "dept", 5, []byte("real-random-salt-value"))
	if err != nil {
		t.Fatalf("build trait update: %v", err)
	}
	assertRealSignature(t, trait)
	if err := runOne(p, trait); err != nil {
		t.Fatalf("expected the real pipeline to accept a trait update against a real NFT: %v", err)
	}

	nft, found, err := deps.Store.GetNFT(target)
	if err != nil || !found {
		t.Fatalf("expected the target nft to still exist: found=%v err=%v", found, err)
	}
	want := txbuilder.ComputeTraitDeltaCommitment("dept", 5, []byte("real-random-salt-value")).String()
	if got := nft.Traits["dept_last_delta_commitment"]; got != want {
		t.Fatalf("expected the recorded delta commitment to match, got %q want %q", got, want)
	}
}

func TestNFTTraitRejectsUnknownTargetByRealPipeline(t *testing.T) {
	p, _ := newRealPipeline(t, tx.Deps{})
	b := newIdentity(t)

	trait, err := b.NFTTrait(types.NFTID{0x99}, "dept", 1, []byte("salt"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := runOne(p, trait); err == nil {
		t.Fatalf("expected the real pipeline to reject a trait update against a nonexistent NFT")
	}
}

func TestNFTTraitRejectsEmptyKey(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.NFTTrait(types.NFTID{0x1}, "", 1, []byte("salt")); err == nil {
		t.Fatalf("expected an empty key to be rejected")
	}
}

func TestNFTTraitRejectsEmptySalt(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.NFTTrait(types.NFTID{0x1}, "dept", 1, nil); err == nil {
		t.Fatalf("expected an empty salt to be rejected")
	}
}

func TestComputeTraitDeltaCommitmentIsDeterministicAndSaltSensitive(t *testing.T) {
	a := txbuilder.ComputeTraitDeltaCommitment("dept", 5, []byte("salt-a"))
	b := txbuilder.ComputeTraitDeltaCommitment("dept", 5, []byte("salt-a"))
	if a != b {
		t.Fatalf("expected identical inputs to produce identical commitments")
	}
	c := txbuilder.ComputeTraitDeltaCommitment("dept", 5, []byte("salt-b"))
	if a == c {
		t.Fatalf("expected a different salt to produce a different commitment")
	}
}
