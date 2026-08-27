package tx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
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
	results := p.ProcessBatch([]tx.Entry{{Tx: voteTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected vote to succeed: %v", results[0].Error)
	}
	record, found, err := deps.Store.GetProposal("proposal-42")
	if err != nil || !found {
		t.Fatalf("expected a proposal record: found=%v err=%v", found, err)
	}
	if len(record.Commitments) != 1 || record.Commitments[0] != (types.Hash{1, 2, 3}) {
		t.Fatalf("unexpected proposal commitments: %+v", record.Commitments)
	}

	// A second ballot on the same proposal accumulates.
	voteTx2 := mustSign(t, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-42"),
			Commitment: types.Hash{4, 5, 6},
		},
	})
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
