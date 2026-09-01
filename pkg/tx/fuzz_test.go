package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// FuzzStage2WellFormednessNeverPanics targets Stage 2's real, fully
// attacker-controlled byte fields (Memo, Sig, SignerPubKey, ClassicalSig,
// ClassicalPubKey) exactly as they arrive over the wire inside a
// types.ShieldedTx, before any cryptographic check has run against them.
// Spec 23's own risk register names "fuzz of the prover/verifier pair" as
// a Year-1 mitigation for circuit bugs; this extends the same discipline
// to the pipeline boundary that receives raw, adversarial transaction
// bytes first. A malformed combination of these fields must always be
// cleanly rejected by the real pipeline.ProcessBatch, never panic —
// regardless of length, content, or whether the optional classical
// dual-sign fields are even present.
func FuzzStage2WellFormednessNeverPanics(f *testing.F) {
	deps := newDeps(f)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		f.Fatalf("generate dilithium key: %v", err)
	}
	classicalPK, classicalSK, err := crypto.GenerateClassicalKey()
	if err != nil {
		f.Fatalf("generate classical key: %v", err)
	}

	atr := decimal.MustFromString("2000")
	base := types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			Asset:     types.AssetSFG,
			ATRUSD:    atr,
			BufferUSD: decimal.MustFromString("2.5").Mul(atr),
		},
	}
	base.TxID = types.ComputeTxID(base.Proof, base.Commitments, base.Nullifier)
	realSig, err := crypto.DilithiumSign(sk, base.TxID[:])
	if err != nil {
		f.Fatalf("dilithium sign: %v", err)
	}
	realClassicalSig, err := crypto.ClassicalSign(classicalSK, base.TxID[:])
	if err != nil {
		f.Fatalf("classical sign: %v", err)
	}

	// A genuinely well-formed, fully dual-signed baseline.
	f.Add([]byte("hello memo"), []byte(realSig), []byte(pk), []byte(realClassicalSig), []byte(classicalPK))
	// Every field empty.
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, []byte{})
	// A real, single (non-dual) signature with no classical fields at all.
	f.Add([]byte{}, []byte(realSig), []byte(pk), []byte{}, []byte{})
	// Every field a single zero byte.
	f.Add([]byte{0x00}, []byte{0x00}, []byte{0x00}, []byte{0x00}, []byte{0x00})
	// A real signature in the wrong field (Memo), everything else real.
	f.Add([]byte(realSig), []byte(realSig), []byte(pk), []byte(realClassicalSig), []byte(classicalPK))
	// A truncated real signature.
	f.Add([]byte("memo"), []byte(realSig)[:len(realSig)/2], []byte(pk), []byte(realClassicalSig), []byte(classicalPK))
	// A classical signature present without its matching pubkey.
	f.Add([]byte("memo"), []byte(realSig), []byte(pk), []byte(realClassicalSig), []byte{})
	// A classical pubkey present without its matching signature.
	f.Add([]byte("memo"), []byte(realSig), []byte(pk), []byte{}, []byte(classicalPK))

	f.Fuzz(func(t *testing.T, memo, sig, signerPK, classicalSig, classicalPK []byte) {
		txn := base
		txn.Memo = memo
		txn.Sig = types.DilithiumSig(sig)
		txn.SignerPubKey = signerPK
		txn.ClassicalSig = classicalSig
		txn.ClassicalPubKey = classicalPK
		// Deliberately not asserting on the returned error itself — only
		// that arbitrary byte combinations across every attacker-controlled
		// Stage 2 field never panic. Go's fuzzing engine already treats a
		// panic during this call as a failure; a returned error (the
		// overwhelmingly likely outcome for anything but the real seeded
		// baseline) is the correct, safe behavior this proves.
		_ = p.ProcessBatch([]tx.Entry{{Tx: txn, SubmittedAt: time.Now()}})
	})
}
