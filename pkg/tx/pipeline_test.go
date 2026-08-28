package tx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// zkSystem is expensive to Setup (~1.5s); build it once for the whole
// package's tests.
var (
	zkOnce sync.Once
	zkSys  *zk.System
	zkErr  error
)

func getZKSystem(t *testing.T) *zk.System {
	t.Helper()
	zkOnce.Do(func() { zkSys, zkErr = zk.Setup() })
	if zkErr != nil {
		t.Fatalf("zk setup: %v", zkErr)
	}
	return zkSys
}

func openStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("pipeline-test-key-32-bytes-pad!!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newDeps(t *testing.T) tx.Deps {
	return tx.Deps{
		Store:     openStore(t),
		StateTree: state.NewMerkleTree(),
		ZK:        getZKSystem(t),
		Vault:     vault.New(vault.DefaultSplits()),
	}
}

// seedVoterNFT gives signerPubKey a real, minted ValidatorNFT directly in
// deps' store — real voter eligibility (pkg/tx's own requireEligibleVoter)
// is unconditional, so any test exercising TxVote/TxVoteReveal behavior
// other than eligibility itself needs this first, exactly the way a real
// wallet would need a real Kind NFTMint to have succeeded beforehand.
func seedVoterNFT(t *testing.T, deps tx.Deps, signerPubKey []byte) {
	t.Helper()
	owner := types.AddressFromPubkey(signerPubKey)
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(owner[:])), Owner: owner}); err != nil {
		t.Fatalf("seed voter nft: %v", err)
	}
}

// mustSign computes t's TxID from Hash(proof || commitments || nullifier)
// (spec 4.1) and produces a real Dilithium signature over it, so every
// test transaction passes Stage 2's actual cryptographic checks rather
// than a placeholder byte string.
func mustSign(t *testing.T, in types.ShieldedTx) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

// buildValidTransfer produces a fully valid, provable, correctly-signed
// Transfer ShieldedTx: two spent notes (60+40) split into a 70 payment +
// 25 change note with a fee of 5.
func buildValidTransfer(t *testing.T) types.ShieldedTx {
	t.Helper()
	sys := getZKSystem(t)
	ztree := zk.NewTree()

	mk := func(v uint64) zk.NoteSecret {
		sk, err := zk.NewSpendKey()
		if err != nil {
			t.Fatal(err)
		}
		rho, err := zk.NewRho()
		if err != nil {
			t.Fatal(err)
		}
		return zk.NoteSecret{Value: v, OwnerSK: sk, Rho: rho}
	}
	in0, in1 := mk(60), mk(40)
	idx0, err := ztree.Insert(in0.Commitment())
	if err != nil {
		t.Fatal(err)
	}
	idx1, err := ztree.Insert(in1.Commitment())
	if err != nil {
		t.Fatal(err)
	}
	proof0, err := ztree.Prove(idx0)
	if err != nil {
		t.Fatal(err)
	}
	proof1, err := ztree.Prove(idx1)
	if err != nil {
		t.Fatal(err)
	}
	out0, out1 := mk(70), mk(25)

	input := zk.TransferInput{
		MerkleRoot: proof0.Root,
		Fee:        5,
		InSecrets:  []zk.NoteSecret{in0, in1},
		InProofs:   []zk.Proof{proof0, proof1},
		OutSecrets: []zk.NoteSecret{out0, out1},
	}
	zproof, err := sys.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(zproof)
	if err != nil {
		t.Fatalf("serialize proof: %v", err)
	}

	pub := input.Public()
	txPub := &types.TransferPublicInputs{
		MerkleRoot: types.Hash(zk.ToBytes32(pub.MerkleRoot)),
		FeeAmount:  pub.Fee,
	}
	for _, n := range pub.Nullifiers {
		txPub.Nullifiers = append(txPub.Nullifiers, types.Hash(zk.ToBytes32(n)))
	}
	for _, c := range pub.OutCommits {
		txPub.OutCommits = append(txPub.OutCommits, types.Hash(zk.ToBytes32(c)))
	}

	return mustSign(t, types.ShieldedTx{
		Nullifier:            txPub.Nullifiers[0],
		Commitments:          txPub.OutCommits,
		Proof:                proofBytes,
		FeeCommit:            types.SumHash([]byte("fee")),
		Kind:                 types.TxTransfer,
		TransferPublicInputs: txPub,
	})
}

func TestPipelineHappyPathTransfer(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("expected success, got error: %v", results[0].Error)
	}
	if !results[0].Tx.StageHints.Complete() {
		t.Fatalf("expected all 5 stages to be marked complete, got %08b", results[0].Tx.StageHints)
	}

	for _, n := range shieldedTx.TransferPublicInputs.Nullifiers {
		spent, err := deps.Store.IsNullifierSpent(n)
		if err != nil || !spent {
			t.Fatalf("expected nullifier %s to be committed spent: spent=%v err=%v", n, spent, err)
		}
	}
}

// TestPipelineCommittedTransferCollectsFeeIntoVault proves Stage 5 routes
// a committed Transfer's fee into the Vault's real 20/10/10/60 split
// (spec 9.2) — using buildValidTransfer's real ZK-proven FeeAmount (5),
// not a fabricated number. A rejected transfer must not touch the Vault
// at all (its fee was never actually collected).
func TestPipelineCommittedTransferCollectsFeeIntoVault(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	wantFee := decimal.FromInt(int64(shieldedTx.TransferPublicInputs.FeeAmount))
	if wantFee.Sign() <= 0 {
		t.Fatalf("test fixture must carry a positive fee, got %s", wantFee)
	}

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected success, got error: %v", results[0].Error)
	}

	splits := vault.DefaultSplits()
	wantEpochBonus := wantFee.Mul(splits.EpochBonus)
	wantAudit := wantFee.Mul(splits.Audit)
	wantRemainder := wantFee.Mul(splits.Remainder)
	wantBurn := wantFee.Mul(splits.Burn)

	v := deps.Vault
	if v.EpochBonusPool.Cmp(wantEpochBonus) != 0 {
		t.Fatalf("epoch bonus pool: got %s want %s", v.EpochBonusPool, wantEpochBonus)
	}
	if v.AuditPool.Cmp(wantAudit) != 0 {
		t.Fatalf("audit pool: got %s want %s", v.AuditPool, wantAudit)
	}
	if v.RemainderPool.Cmp(wantRemainder) != 0 {
		t.Fatalf("remainder pool: got %s want %s", v.RemainderPool, wantRemainder)
	}
	if v.BurnedTotal.Cmp(wantBurn) != 0 {
		t.Fatalf("burned total: got %s want %s", v.BurnedTotal, wantBurn)
	}
}

// TestPipelineRejectedTransferDoesNotCollectFee proves a Transfer that
// fails a later stage never has its fee collected — the Vault must stay
// untouched, matching the pipeline's atomicity rule (spec 5.3: rejection
// leaves no trait or balance changes).
func TestPipelineRejectedTransferDoesNotCollectFee(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	shieldedTx = mustSign(t, types.ShieldedTx{
		Kind:                 types.TxTransfer,
		TransferPublicInputs: shieldedTx.TransferPublicInputs,
		FeeCommit:            shieldedTx.FeeCommit,
		Proof:                []byte("tampered proof, must fail stage 1 verification"),
	})

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected the tampered transfer to be rejected")
	}

	v := deps.Vault
	if v.EpochBonusPool.Sign() != 0 || v.AuditPool.Sign() != 0 || v.RemainderPool.Sign() != 0 || v.BurnedTotal.Sign() != 0 {
		t.Fatalf("expected the Vault to be untouched by a rejected transfer, got %+v", v)
	}
}

func TestPipelineDoubleSpendWithinBatchRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	t1 := buildValidTransfer(t)
	t2 := t1 // exact duplicate: same nullifiers

	results := p.ProcessBatch([]tx.Entry{
		{Tx: t1, SubmittedAt: time.Now()},
		{Tx: t2, SubmittedAt: time.Now()},
	})
	successes := 0
	for _, r := range results {
		if r.Error == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 of 2 duplicate transfers to succeed, got %d", successes)
	}
}

func TestPipelineReplayAfterCommitRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)

	r1 := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if r1[0].Error != nil {
		t.Fatalf("first submission should succeed: %v", r1[0].Error)
	}
	r2 := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if r2[0].Error == nil {
		t.Fatalf("expected replay of an already-committed transfer to fail")
	}
}

func TestPipelineAtomicityReleasesNullifierOnLaterStageFailure(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	shieldedTx.FeeCommit = types.Hash{} // forces a Stage 2 well-formedness failure (missing fee commitment) without touching the signature

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected stage 2 rejection for a missing fee commitment")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 2 {
		t.Fatalf("expected a stage-2 StageError, got %v", results[0].Error)
	}

	// The nullifier must have been released, not stuck pending.
	spent, err := deps.Store.IsNullifierSpent(shieldedTx.TransferPublicInputs.Nullifiers[0])
	if err != nil {
		t.Fatalf("spent lookup: %v", err)
	}
	if spent {
		t.Fatalf("a rejected transaction must not leave its nullifier marked spent")
	}
}

func TestPipelineTamperedSignatureRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	shieldedTx.Sig[0] ^= 0xFF // flip a bit: signature no longer verifies

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a tampered signature to be rejected")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 2 {
		t.Fatalf("expected a stage-2 StageError, got %v", results[0].Error)
	}
}

func TestPipelineTamperedTxIDRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	shieldedTx.TxID[0] ^= 0xFF // no longer matches Hash(proof||commitments||nullifier)

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a tampered TxID to be rejected")
	}
}

func TestPipelineMissingSignerPubKeyRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)
	shieldedTx.SignerPubKey = nil

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a missing signer public key to be rejected")
	}
}

func TestPipelineExpiredTxRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	shieldedTx := buildValidTransfer(t)

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now().Add(-2 * tx.TxTTL)}})
	if results[0].Error == nil {
		t.Fatalf("expected an expired transaction to be rejected at stage 2")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 2 {
		t.Fatalf("expected a stage-2 StageError, got %v (%T)", results[0].Error, results[0].Error)
	}
}

// TestPipelineOversizedTxRejected proves Stage 2 real-cryptographically
// rejects a single transaction too large to ever safely fit in a batch
// (MaxTxSize), rather than deferring it forever to Mempool.DrainBatchBytes
// (which drains at least one entry no matter its size, precisely so it
// never wedges the queue — the actual rejection has to happen here).
func TestPipelineOversizedTxRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	hugeTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-huge"),
			Commitment: types.Hash{1},
		},
		Memo: make([]byte, tx.MaxTxSize+1),
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: hugeTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected an oversized transaction to be rejected at stage 2")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 2 {
		t.Fatalf("expected a stage-2 StageError, got %v (%T)", results[0].Error, results[0].Error)
	}
}

func TestPipelineBankDepositBufferMismatchRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	bankTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         decimal.MustFromString("2000"),
			BufferUSD:      decimal.MustFromString("999"), // wrong: should be 2.5*2000=5000
		},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected buffer-mismatch rejection at stage 4")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError, got %v", results[0].Error)
	}
}

func TestPipelineBankDepositCorrectBufferAccepted(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	bankTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         decimal.MustFromString("2000"),
			BufferUSD:      decimal.MustFromString("5000"), // 2.5 * 2000
		},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected correct buffer to be accepted, got %v", results[0].Error)
	}
}

// bankDepositTx builds a self-consistent (buffer = 2.5*ATR) signed
// BankDeposit tx claiming the given price/ATR for asset, so oracle-check
// tests exercise Stage 4's oracle cross-check in isolation from its
// separate internal buffer-consistency check.
func bankDepositTx(t *testing.T, asset types.AssetID, priceUSD, atrUSD string) types.ShieldedTx {
	t.Helper()
	price := decimal.MustFromString(priceUSD)
	atr := decimal.MustFromString(atrUSD)
	return mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			Asset:          asset,
			OraclePriceUSD: price,
			ATRUSD:         atr,
			BufferUSD:      bank.DepositATRMultiple.Mul(atr),
		},
	})
}

func TestPipelineBankDepositWithinOracleToleranceAccepted(t *testing.T) {
	deps := newDeps(t)
	deps.Oracle = oracle.NewQuorum(decimal.MustFromString("0.05"), oracle.StaticSource{
		Value: oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")},
	})
	p := tx.NewPipeline(deps)
	// 1% off the real 60000/2000 reading, within the default 2% tolerance.
	bankTx := bankDepositTx(t, types.AssetBTC, "60500", "2010")
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected a within-tolerance claimed price to be accepted, got %v", results[0].Error)
	}
}

func TestPipelineBankDepositBeyondOracleToleranceRejected(t *testing.T) {
	deps := newDeps(t)
	deps.Oracle = oracle.NewQuorum(decimal.MustFromString("0.05"), oracle.StaticSource{
		Value: oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")},
	})
	p := tx.NewPipeline(deps)
	// Claims a price 10x the real oracle reading — exactly the exploit
	// this wiring closes: internally self-consistent (buffer = 2.5*ATR)
	// but not tied to reality at all.
	bankTx := bankDepositTx(t, types.AssetBTC, "600000", "20000")
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a claimed price wildly diverging from the real oracle reading to be rejected")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError, got %v", results[0].Error)
	}
}

func TestPipelineBankDepositFrozenOnOracleDisagreement(t *testing.T) {
	deps := newDeps(t)
	// Two sources that disagree far beyond the quorum's own bound: Quote
	// itself returns ErrDisagreement, simulating spec 11.3's freeze
	// condition — a real, live disagreement between independent feeds,
	// not a single feed simply being unreachable.
	deps.Oracle = oracle.NewQuorum(decimal.MustFromString("0.02"),
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")}},
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: decimal.MustFromString("90000"), ATRUSD: decimal.MustFromString("2000")}},
	)
	p := tx.NewPipeline(deps)
	bankTx := bankDepositTx(t, types.AssetBTC, "60000", "2000")
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a new deposit to be frozen while the oracle quorum disagrees (spec 11.3)")
	}
}

func TestPipelineBankWithdrawUsesLastGoodOnOracleDisagreement(t *testing.T) {
	deps := newDeps(t)
	agree := oracle.StaticSource{Value: oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")}}
	// A source whose Quote call itself can be toggled from agreeing to
	// disagreeing between the two ProcessBatch calls below, so the same
	// Quorum instance both records a real last-good snapshot and then
	// genuinely disagrees.
	toggle := &toggleSource{agree: oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")}}
	q := oracle.NewQuorum(decimal.MustFromString("0.02"), agree, toggle)
	deps.Oracle = q
	p := tx.NewPipeline(deps)

	// First: sources agree, priming Quorum's LastGood snapshot for BTC.
	seed := bankDepositTx(t, types.AssetBTC, "60000", "2000")
	if res := p.ProcessBatch([]tx.Entry{{Tx: seed, SubmittedAt: time.Now()}}); res[0].Error != nil {
		t.Fatalf("expected the seeding deposit to be accepted, got %v", res[0].Error)
	}

	// Now the sources disagree wildly. A new deposit must be frozen...
	toggle.disagree = true
	depositTx := bankDepositTx(t, types.AssetBTC, "60000", "2000")
	if res := p.ProcessBatch([]tx.Entry{{Tx: depositTx, SubmittedAt: time.Now()}}); res[0].Error == nil {
		t.Fatalf("expected a new deposit to be frozen during disagreement")
	}

	// ...but a withdrawal pricing against the same pre-disagreement figures
	// must still be accepted via LastGood (spec 11.3: "use last-good
	// snapshots for open holds").
	atr := decimal.MustFromString("2000")
	withdrawTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankWithdraw,
		BankPublicInputs: &types.BankPublicInputs{
			Asset:          types.AssetBTC,
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         atr,
			BufferUSD:      bank.WithdrawATRMultiple.Mul(atr),
		},
	})
	if res := p.ProcessBatch([]tx.Entry{{Tx: withdrawTx, SubmittedAt: time.Now()}}); res[0].Error != nil {
		t.Fatalf("expected a withdrawal to fall back to the last-good oracle snapshot during disagreement, got %v", res[0].Error)
	}
}

// toggleSource lets a test flip one Source from agreeing with another
// StaticSource to disagreeing with it, so the same oracle.Quorum can be
// driven through both a real agreement (priming LastGood) and a real
// disagreement (ErrDisagreement) without rebuilding it.
type toggleSource struct {
	agree    oracle.Quote
	disagree bool
}

func (s *toggleSource) Quote(types.AssetID) (oracle.Quote, error) {
	if s.disagree {
		return oracle.Quote{PriceUSD: decimal.MustFromString("999999"), ATRUSD: decimal.MustFromString("999999")}, nil
	}
	return s.agree, nil
}

func TestPipelineContainerSyncShadowMismatchBlocksCommit(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	id := types.ID("acme-container")

	first := mustSign(t, types.ShieldedTx{
		Kind: types.TxContainerSync, ContainerID: &id, Commitments: []types.Hash{{9, 9, 9}},
	})
	r1 := p.ProcessBatch([]tx.Entry{{Tx: first, SubmittedAt: time.Now()}})
	if r1[0].Error != nil {
		t.Fatalf("first sync should succeed: %v", r1[0].Error)
	}

	claimedShadow := types.Hash{5, 5, 5}
	second := mustSign(t, types.ShieldedTx{
		Kind: types.TxContainerSync, ContainerID: &id, Commitments: []types.Hash{{1, 1, 1}},
		Memo: claimedShadow[:], // claims a duplicate-server digest that won't match {1,1,1}
	})
	r2 := p.ProcessBatch([]tx.Entry{{Tx: second, SubmittedAt: time.Now()}})
	if r2[0].Error == nil {
		t.Fatalf("expected shadow-verification mismatch to block commit")
	}
	se, ok := r2[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError, got %v", r2[0].Error)
	}
}

// TestPipelineContainerSyncMissingShadowMemoBlocksCommit is a direct
// regression test for a real bypass live auditing surfaced: shadow
// verification (spec 16.3) only ran when a submitter happened to include
// a 32-byte Memo, so simply omitting it skipped verification entirely
// instead of being rejected — the opposite of "mismatch blocks commit."
func TestPipelineContainerSyncMissingShadowMemoBlocksCommit(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	id := types.ID("acme-container-no-memo")

	first := mustSign(t, types.ShieldedTx{
		Kind: types.TxContainerSync, ContainerID: &id, Commitments: []types.Hash{{9, 9, 9}},
	})
	r1 := p.ProcessBatch([]tx.Entry{{Tx: first, SubmittedAt: time.Now()}})
	if r1[0].Error != nil {
		t.Fatalf("first sync should succeed: %v", r1[0].Error)
	}

	second := mustSign(t, types.ShieldedTx{
		Kind: types.TxContainerSync, ContainerID: &id, Commitments: []types.Hash{{1, 1, 1}},
		// No Memo at all — this must be rejected, not silently accepted.
	})
	r2 := p.ProcessBatch([]tx.Entry{{Tx: second, SubmittedAt: time.Now()}})
	if r2[0].Error == nil {
		t.Fatalf("expected a second sync with no shadow-verification memo to be rejected")
	}
	se, ok := r2[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError, got %v", r2[0].Error)
	}
}

func TestPipelineNFTTraitMissingTargetRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	traitTx := mustSign(t, types.ShieldedTx{
		Kind:              types.TxNFTTrait,
		Commitments:       []types.Hash{{7}},
		TraitPublicInputs: &types.TraitPublicInputs{Key: "balance", DeltaCommitment: types.Hash{8}},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: traitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected rejection for a trait update against a non-existent NFT")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError (NFT not found), got %v", results[0].Error)
	}
}

// TestPipelineVoteRejectsUnmintedWallet is a direct regression test for
// the real Sybil-voting gap this session closed: a freshly generated
// keypair with no minted ValidatorNFT must not be able to cast a ballot
// at all, let alone single-handedly pass a proposal. Before
// requireEligibleVoter existed, "voter" was derived from nothing but the
// signer's own public key, so ANY number of throwaway keypairs could
// each cast one self-approving vote for free.
func TestPipelineVoteRejectsUnmintedWallet(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)

	voteTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("sybil-proposal"),
			Commitment: types.Hash{1},
		},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: voteTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a vote from a wallet with no minted NFT to be rejected")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError (voter eligibility), got %v", results[0].Error)
	}

	// The real attack this closes: a flood of distinct, freshly generated
	// keypairs each attempting to self-approve — every single one must be
	// rejected, and none of their ballots ever get recorded, proving a
	// Sybil flood costs an attacker nothing but also gains them nothing.
	for i := 0; i < 5; i++ {
		sybilTx := mustSign(t, types.ShieldedTx{
			Kind: types.TxVote,
			VotePublicInputs: &types.VotePublicInputs{
				ProposalID: types.ID("sybil-proposal"),
				Commitment: types.Hash{byte(i + 2)},
			},
		})
		if r := p.ProcessBatch([]tx.Entry{{Tx: sybilTx, SubmittedAt: time.Now()}}); r[0].Error == nil {
			t.Fatalf("expected sybil attempt %d (fresh unminted keypair) to be rejected", i)
		}
	}

	if _, found, err := deps.Store.GetProposal("sybil-proposal"); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected no proposal record to exist at all — every attempted ballot was ineligible")
	}
}

func TestPipelineVoteRecordsBallotCommitment(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	voteTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-42"),
			Commitment: types.Hash{1, 2, 3},
		},
	})
	seedVoterNFT(t, deps, voteTx.SignerPubKey)
	voter1 := types.NFTID(types.SumHash(voteTx.SignerPubKey))
	results := p.ProcessBatch([]tx.Entry{{Tx: voteTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected vote to succeed: %v", results[0].Error)
	}
	record, found, err := deps.Store.GetProposal("proposal-42")
	if err != nil || !found {
		t.Fatalf("expected a proposal record: found=%v err=%v", found, err)
	}
	if len(record.Commitments) != 1 || record.Commitments[voter1] != (types.Hash{1, 2, 3}) {
		t.Fatalf("unexpected proposal commitments: %+v", record.Commitments)
	}

	// A second ballot from a different voter on the same proposal accumulates.
	voteTx2 := mustSign(t, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-42"),
			Commitment: types.Hash{4, 5, 6},
		},
	})
	seedVoterNFT(t, deps, voteTx2.SignerPubKey)
	results2 := p.ProcessBatch([]tx.Entry{{Tx: voteTx2, SubmittedAt: time.Now()}})
	if results2[0].Error != nil {
		t.Fatalf("expected second vote to succeed: %v", results2[0].Error)
	}
	record, _, err = deps.Store.GetProposal("proposal-42")
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if len(record.Commitments) != 2 {
		t.Fatalf("expected 2 accumulated commitments, got %d", len(record.Commitments))
	}
}

// TestPipelineVoteRejectsDoubleVoteFromSameVoter proves "one NFT, one
// vote" (spec 9.1) is a real, enforced check, not just documented intent.
func TestPipelineVoteRejectsDoubleVoteFromSameVoter(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk)
	first := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-double"),
			Commitment: types.Hash{1},
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: first, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the first ballot to succeed: %v", r[0].Error)
	}

	second := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-double"),
			Commitment: types.Hash{2},
		},
	})
	r := p.ProcessBatch([]tx.Entry{{Tx: second, SubmittedAt: time.Now()}})
	if r[0].Error == nil {
		t.Fatalf("expected a second ballot from the same voter to be rejected")
	}
}

// TestPipelineVoteRevealMatchingCommitmentRecordsChoice proves a reveal
// that genuinely reproduces its earlier commitment (via
// types.ComputeVoteCommitment, the real formula, not a stand-in) is
// accepted and recorded, and that reveal is checked cryptographically:
// the wrong Approve or the wrong Nonce is rejected, not merely logged.
func TestPipelineVoteRevealMatchingCommitmentRecordsChoice(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk)
	voter := types.NFTID(types.SumHash([]byte(pk)))
	nonce := types.Hash{7, 7, 7}
	commitment := types.ComputeVoteCommitment(voter, true, nonce)

	commitTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-reveal"),
			Commitment: commitment,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the commit to succeed: %v", r[0].Error)
	}

	// Wrong Approve: same nonce, flipped choice — must not match.
	wrongApprove := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("proposal-reveal"),
			Approve:    false,
			Nonce:      nonce,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: wrongApprove, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a reveal with the wrong Approve to be rejected")
	}

	// Wrong nonce: correct choice, wrong nonce — must not match either.
	wrongNonce := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("proposal-reveal"),
			Approve:    true,
			Nonce:      types.Hash{9, 9, 9},
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: wrongNonce, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a reveal with the wrong nonce to be rejected")
	}

	// The genuine reveal succeeds and is recorded.
	reveal := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("proposal-reveal"),
			Approve:    true,
			Nonce:      nonce,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: reveal, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the genuine reveal to succeed: %v", r[0].Error)
	}

	record, found, err := deps.Store.GetProposal("proposal-reveal")
	if err != nil || !found {
		t.Fatalf("expected a proposal record: found=%v err=%v", found, err)
	}
	approve, revealed := record.Reveals[voter]
	if !revealed || !approve {
		t.Fatalf("expected the voter's Approve=true reveal to be recorded, got revealed=%v approve=%v", revealed, approve)
	}

	// A second reveal attempt (even a genuine one) is rejected — a voter
	// only gets to open their ballot once.
	if r := p.ProcessBatch([]tx.Entry{{Tx: reveal, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a second reveal from the same voter to be rejected")
	}
}

// TestTallyDueProposalsCountsRevealedBallots proves the epoch-boundary
// tally (TallyDueProposals) is real: it counts only genuinely revealed
// ballots via governance.Tally, decides Passed by real majority, persists
// the outcome, is idempotent (a second call doesn't re-tally or
// double-count), and leaves a still-open (same-epoch) proposal alone.
func TestTallyDueProposalsCountsRevealedBallots(t *testing.T) {
	deps := newDeps(t)
	deps.Epoch = 1
	p := tx.NewPipeline(deps)

	cast := func(approve bool) {
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("generate signer key: %v", err)
		}
		seedVoterNFT(t, deps, pk)
		voter := types.NFTID(types.SumHash([]byte(pk)))
		nonce := types.Hash{byte(len(pk))}
		commitment := types.ComputeVoteCommitment(voter, approve, nonce)
		commitTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
			Kind:             types.TxVote,
			VotePublicInputs: &types.VotePublicInputs{ProposalID: types.ID("tally-proposal"), Commitment: commitment},
		})
		if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
			t.Fatalf("commit: %v", r[0].Error)
		}
		revealTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
			Kind: types.TxVoteReveal,
			VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
				ProposalID: types.ID("tally-proposal"), Approve: approve, Nonce: nonce,
			},
		})
		if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
			t.Fatalf("reveal: %v", r[0].Error)
		}
	}
	cast(true)
	cast(true)
	cast(false)

	// Still epoch 1: not due yet.
	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally at same epoch: %v", err)
	}
	if len(tallied) != 0 {
		t.Fatalf("expected no proposals due while still in their own epoch, got %d", len(tallied))
	}

	// Epoch has moved on: due now.
	tallied, err = p.TallyDueProposals(2)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 {
		t.Fatalf("expected exactly 1 proposal tallied, got %d", len(tallied))
	}
	if tallied[0].Approve != 2 || tallied[0].Reject != 1 || !tallied[0].Passed {
		t.Fatalf("unexpected tally result: %+v", tallied[0])
	}

	record, found, err := deps.Store.GetProposal("tally-proposal")
	if err != nil || !found {
		t.Fatalf("expected a persisted proposal record: found=%v err=%v", found, err)
	}
	if !record.Tallied || record.Approve != 2 || record.Reject != 1 || !record.Passed {
		t.Fatalf("unexpected persisted tally: %+v", record)
	}

	// Idempotent: tallying again must not re-count or change the result.
	tallied, err = p.TallyDueProposals(3)
	if err != nil {
		t.Fatalf("second tally: %v", err)
	}
	if len(tallied) != 0 {
		t.Fatalf("expected an already-tallied proposal to be skipped, got %d", len(tallied))
	}

	// Voting/revealing on an already-tallied proposal is closed — real
	// eligibility for this new key first, so the rejection below
	// genuinely exercises the "already tallied" check this test is
	// about, not just any rejection.
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk)
	lateVote := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind:             types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{ProposalID: types.ID("tally-proposal"), Commitment: types.Hash{1}},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: lateVote, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a vote on an already-tallied proposal to be rejected")
	}
}

// TestTallyDueProposalsAppliesPassedParamChange proves a passed
// ProposalParamChange vote has a real, live effect — not just a persisted
// tally outcome: Deps.Governance is mutated in place, and a
// BankDeposit's Stage 4 buffer check immediately starts enforcing the new
// multiplier rather than the old one, in the very next batch.
func TestTallyDueProposalsAppliesPassedParamChange(t *testing.T) {
	deps := newDeps(t)
	deps.Epoch = 1
	govParams := governance.Default()
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)

	// First voter's TxVote binds the proposal to a concrete change:
	// DepositATRMultiple 2.5 -> 4.
	pk1, sk1, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk1)
	voter1 := types.NFTID(types.SumHash([]byte(pk1)))
	nonce1 := types.Hash{1}
	commit1 := mustSignWithKey(t, pk1, sk1, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("raise-deposit-atr"),
			Commitment: types.ComputeVoteCommitment(voter1, true, nonce1),
			ParamKey:   "DepositATRMultiple",
			NewValue:   "4",
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: commit1, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("commit1: %v", r[0].Error)
	}
	reveal1 := mustSignWithKey(t, pk1, sk1, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("raise-deposit-atr"), Approve: true, Nonce: nonce1,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: reveal1, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("reveal1: %v", r[0].Error)
	}

	// A second voter approves too (majority), WITHOUT specifying
	// ParamKey/NewValue — proving only the first voter's binding sticks,
	// not a later one silently overriding it.
	pk2, sk2, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk2)
	voter2 := types.NFTID(types.SumHash([]byte(pk2)))
	nonce2 := types.Hash{2}
	commit2 := mustSignWithKey(t, pk2, sk2, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("raise-deposit-atr"),
			Commitment: types.ComputeVoteCommitment(voter2, true, nonce2),
			ParamKey:   "SomethingElseEntirely",
			NewValue:   "999",
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: commit2, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("commit2: %v", r[0].Error)
	}
	reveal2 := mustSignWithKey(t, pk2, sk2, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("raise-deposit-atr"), Approve: true, Nonce: nonce2,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: reveal2, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("reveal2: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(2)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed {
		t.Fatalf("expected the proposal to pass 2-0, got %+v", tallied)
	}
	if !tallied[0].Applied {
		t.Fatalf("expected the param change to be marked Applied")
	}
	if deps.Governance.DepositATRMultiple.Cmp(decimal.MustFromString("4")) != 0 {
		t.Fatalf("expected live DepositATRMultiple to become 4, got %s", deps.Governance.DepositATRMultiple)
	}
	if tallied[0].ParamKey != "DepositATRMultiple" {
		t.Fatalf("expected the FIRST voter's ParamKey binding to stick, got %q", tallied[0].ParamKey)
	}

	// The real, live proof: a BankDeposit whose bound buffer matches the
	// OLD multiplier (2.5) must now be rejected, and one matching the NEW
	// multiplier (4) must be accepted — Stage 4 is reading the just
	// -updated governance state, not a stale snapshot.
	atr := decimal.MustFromString("2000")
	staleTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         atr,
			BufferUSD:      decimal.MustFromString("2.5").Mul(atr),
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: staleTx, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a buffer bound to the pre-vote multiplier (2.5) to now be rejected")
	}

	freshTx := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         atr,
			BufferUSD:      decimal.MustFromString("4").Mul(atr),
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: freshTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected a buffer bound to the new, governance-voted multiplier (4) to be accepted, got %v", r[0].Error)
	}
}

// TestTallyDueProposalsAppliesVaultShareChange proves a passed
// VaultEpochBonusShare (etc.) change resyncs Deps.Vault.Splits for real,
// exercising vault.SplitsFromParams — previously dead code (nothing ever
// called it outside its own package) — from the live tally path.
func TestTallyDueProposalsAppliesVaultShareChange(t *testing.T) {
	deps := newDeps(t)
	deps.Epoch = 1
	govParams := governance.Default()
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk)
	voter := types.NFTID(types.SumHash([]byte(pk)))
	nonce := types.Hash{3}
	commit := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("raise-burn-share"),
			Commitment: types.ComputeVoteCommitment(voter, true, nonce),
			ParamKey:   "VaultBurnShare",
			NewValue:   "0.25",
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: commit, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("commit: %v", r[0].Error)
	}
	reveal := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("raise-burn-share"), Approve: true, Nonce: nonce,
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: reveal, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("reveal: %v", r[0].Error)
	}

	if _, err := p.TallyDueProposals(2); err != nil {
		t.Fatalf("tally: %v", err)
	}

	if deps.Vault.Splits.Burn.Cmp(decimal.MustFromString("0.25")) != 0 {
		t.Fatalf("expected Vault.Splits.Burn to be resynced to 0.25, got %s", deps.Vault.Splits.Burn)
	}

	// Real live effect: CollectFee now routes the new share to BurnedTotal.
	deps.Vault.CollectFee(decimal.FromInt(100))
	if deps.Vault.BurnedTotal.Cmp(decimal.MustFromString("25")) != 0 {
		t.Fatalf("expected CollectFee(100) to burn 25 (25%%), got %s", deps.Vault.BurnedTotal)
	}
}

// mustSignWithKey signs in with a caller-supplied keypair, unlike mustSign
// (which always generates a fresh one) — needed to submit multiple
// transactions from the same real, signature-verified wallet identity.
func mustSignWithKey(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, in types.ShieldedTx) types.ShieldedTx {
	t.Helper()
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

// TestPipelineSpikeDetectionHoldsFloodingWallet proves Stage 2 genuinely
// enforces spec 15.4's spike defense end to end: real signature-verified
// traffic from one wallet is recorded against a real RateMonitor, a burst
// past its baseline is rejected and places a real hold, that hold persists
// even for a follow-up transaction with no further burst, and a different
// wallet's traffic is entirely unaffected.
func TestPipelineSpikeDetectionHoldsFloodingWallet(t *testing.T) {
	deps := newDeps(t)
	deps.Silent = silent.NewRateMonitor()
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	seedVoterNFT(t, deps, pk)
	addr := types.Address(types.SumHash([]byte(pk)))
	deps.Silent.SetBaseline(addr, decimal.FromInt(1)) // 1 tx/min normal

	now := time.Now()
	voteTx := func(commit byte) types.ShieldedTx {
		return mustSignWithKey(t, pk, sk, types.ShieldedTx{
			Kind: types.TxVote,
			VotePublicInputs: &types.VotePublicInputs{
				ProposalID: types.ID("spike-test-proposal"),
				Commitment: types.Hash{commit},
			},
		})
	}

	// First tx: at/under baseline, must be admitted.
	r := p.ProcessBatch([]tx.Entry{{Tx: voteTx(1), SubmittedAt: now}})
	if r[0].Error != nil {
		t.Fatalf("expected the first transaction to be admitted: %v", r[0].Error)
	}

	// Burst well past baseline*1.2 within the same window.
	var lastErr error
	for i := byte(2); i < 10; i++ {
		res := p.ProcessBatch([]tx.Entry{{Tx: voteTx(i), SubmittedAt: now}})
		lastErr = res[0].Error
		if lastErr != nil {
			break
		}
	}
	if lastErr == nil {
		t.Fatalf("expected the burst to trip spike detection and be rejected")
	}

	if !deps.Silent.IsHeld(addr, now) {
		t.Fatalf("expected the flooding wallet to be under a real hold after the spike was flagged")
	}

	// A follow-up transaction, even with no further burst, must still be
	// rejected while the hold is active — the hold is real state, not
	// just a value EvaluateSpike computed and discarded.
	res := p.ProcessBatch([]tx.Entry{{Tx: voteTx(200), SubmittedAt: now.Add(time.Second)}})
	if res[0].Error == nil {
		t.Fatalf("expected a transaction from a held wallet to still be rejected")
	}

	// A different wallet is entirely unaffected.
	otherPK, otherSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate other signer key: %v", err)
	}
	seedVoterNFT(t, deps, otherPK)
	otherTx := mustSignWithKey(t, otherPK, otherSK, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("spike-test-proposal"),
			Commitment: types.Hash{99},
		},
	})
	otherRes := p.ProcessBatch([]tx.Entry{{Tx: otherTx, SubmittedAt: now}})
	if otherRes[0].Error != nil {
		t.Fatalf("expected an unrelated wallet's transaction to be unaffected: %v", otherRes[0].Error)
	}
}

func TestPipelineNFTTraitAppliesToExistingNFT(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	nftID := types.NFTID{7}
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: nftID, Traits: map[string]string{}}); err != nil {
		t.Fatalf("seed nft: %v", err)
	}
	traitTx := mustSign(t, types.ShieldedTx{
		Kind:              types.TxNFTTrait,
		Commitments:       []types.Hash{types.Hash(nftID)},
		TraitPublicInputs: &types.TraitPublicInputs{Key: "balance", DeltaCommitment: types.Hash{8}},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: traitTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected trait update against an existing NFT to succeed: %v", results[0].Error)
	}
}
