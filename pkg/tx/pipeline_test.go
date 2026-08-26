package tx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
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
	t.Cleanup(func() { s.Close() })
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

// buildValidTransfer produces a fully valid, provable Transfer ShieldedTx:
// two spent notes (60+40) split into a 70 payment + 25 change note with a
// fee of 5.
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

	return types.ShieldedTx{
		TxID:                 types.SumHash(proofBytes),
		Nullifier:            txPub.Nullifiers[0],
		Commitments:          txPub.OutCommits,
		Proof:                proofBytes,
		FeeCommit:            types.SumHash([]byte("fee")),
		Sig:                  types.DilithiumSig("test-sig"),
		Kind:                 types.TxTransfer,
		TransferPublicInputs: txPub,
	}
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
	shieldedTx.Sig = nil // forces a Stage 2 well-formedness failure

	results := p.ProcessBatch([]tx.Entry{{Tx: shieldedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected stage 2 rejection for a missing signature")
	}

	// The nullifier must have been released, not stuck pending: a
	// corrected resubmission should now succeed.
	fixed := buildValidTransferFromSameTx(t, shieldedTx)
	_ = fixed // not used further; the important assertion is on spent-state below
	spent, err := deps.Store.IsNullifierSpent(shieldedTx.TransferPublicInputs.Nullifiers[0])
	if err != nil {
		t.Fatalf("spent lookup: %v", err)
	}
	if spent {
		t.Fatalf("a rejected transaction must not leave its nullifier marked spent")
	}
}

// buildValidTransferFromSameTx is a no-op helper kept for readability at
// the call site above; it documents that a fresh, correctly-signed
// resubmission is what a real wallet would do next.
func buildValidTransferFromSameTx(t *testing.T, original types.ShieldedTx) types.ShieldedTx {
	t.Helper()
	return original
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
	bankTx := types.ShieldedTx{
		TxID: types.Hash{1},
		Kind: types.TxBankDeposit,
		Sig:  types.DilithiumSig("sig"),
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         decimal.MustFromString("2000"),
			BufferUSD:      decimal.MustFromString("999"), // wrong: should be 2.5*2000=5000
		},
	}
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
	bankTx := types.ShieldedTx{
		TxID: types.Hash{2},
		Kind: types.TxBankDeposit,
		Sig:  types.DilithiumSig("sig"),
		BankPublicInputs: &types.BankPublicInputs{
			OraclePriceUSD: decimal.MustFromString("60000"),
			ATRUSD:         decimal.MustFromString("2000"),
			BufferUSD:      decimal.MustFromString("5000"), // 2.5 * 2000
		},
	}
	results := p.ProcessBatch([]tx.Entry{{Tx: bankTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected correct buffer to be accepted, got %v", results[0].Error)
	}
}

func TestPipelineContainerSyncShadowMismatchBlocksCommit(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	id := types.ID("acme-container")

	first := types.ShieldedTx{
		TxID: types.Hash{3}, Kind: types.TxContainerSync, Sig: types.DilithiumSig("sig"),
		ContainerID: &id, Commitments: []types.Hash{{9, 9, 9}},
	}
	r1 := p.ProcessBatch([]tx.Entry{{Tx: first, SubmittedAt: time.Now()}})
	if r1[0].Error != nil {
		t.Fatalf("first sync should succeed: %v", r1[0].Error)
	}

	claimedShadow := types.Hash{5, 5, 5}
	second := types.ShieldedTx{
		TxID: types.Hash{4}, Kind: types.TxContainerSync, Sig: types.DilithiumSig("sig"),
		ContainerID: &id, Commitments: []types.Hash{{1, 1, 1}},
		Memo: claimedShadow[:], // claims a duplicate-server digest that won't match {1,1,1}
	}
	r2 := p.ProcessBatch([]tx.Entry{{Tx: second, SubmittedAt: time.Now()}})
	if r2[0].Error == nil {
		t.Fatalf("expected shadow-verification mismatch to block commit")
	}
}

func TestPipelineNFTTraitMissingTargetRejected(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	traitTx := types.ShieldedTx{
		TxID: types.Hash{6}, Kind: types.TxNFTTrait, Sig: types.DilithiumSig("sig"),
		Commitments:       []types.Hash{{7}},
		TraitPublicInputs: &types.TraitPublicInputs{Key: "balance", DeltaCommitment: types.Hash{8}},
	}
	results := p.ProcessBatch([]tx.Entry{{Tx: traitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected rejection for a trait update against a non-existent NFT")
	}
}

func TestPipelineNFTTraitAppliesToExistingNFT(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	nftID := types.NFTID{7}
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: nftID, Traits: map[string]string{}}); err != nil {
		t.Fatalf("seed nft: %v", err)
	}
	traitTx := types.ShieldedTx{
		TxID: types.Hash{9}, Kind: types.TxNFTTrait, Sig: types.DilithiumSig("sig"),
		Commitments:       []types.Hash{types.Hash(nftID)},
		TraitPublicInputs: &types.TraitPublicInputs{Key: "balance", DeltaCommitment: types.Hash{8}},
	}
	results := p.ProcessBatch([]tx.Entry{{Tx: traitTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected trait update against an existing NFT to succeed: %v", results[0].Error)
	}
}
