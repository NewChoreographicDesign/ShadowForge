package shieldedwallet

import (
	"crypto/ecdh"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// ErrInsufficientNotes means this wallet doesn't currently know enough
// spendable notes to build a transfer — either it hasn't discovered
// enough via Sync yet, or (this build's own fixed 2-input circuit shape)
// it has fewer than the 2 notes every transfer needs regardless of how
// small the payment is.
var ErrInsufficientNotes = fmt.Errorf("shieldedwallet: fewer than %d known spendable notes (this build's circuit always spends exactly %d)", zk.NumInputs, zk.NumInputs)

// BuildTransfer constructs a real, fully-proven TxTransfer sending amount
// to receiverPub, paying fee, with genuine change returned to this
// wallet. It selects exactly zk.NumInputs known notes (largest-value
// first, to cover amount+fee in as few notes as this build's fixed
// 2-input shape allows) and produces exactly zk.NumOutputs outputs — a
// real Groth16 proof via sys.Prove, anchored to this wallet's current
// locally-synced canonical root (Sync must have run first for that root
// to mean anything to a real node). Selected inputs are marked spent
// locally immediately (before this method returns, success or failure of
// anything after proving), so a second BuildTransfer call in the same
// process can't try to reuse them — real nullifier-based double-spend
// rejection is still the network's actual authority, this is just
// honest local bookkeeping.
func (w *Wallet) BuildTransfer(sys *zk.System, receiverPub *ecdh.PublicKey, amount, fee uint64) (types.ShieldedTx, error) {
	w.mu.Lock()
	if len(w.notes) < zk.NumInputs {
		w.mu.Unlock()
		return types.ShieldedTx{}, ErrInsufficientNotes
	}
	selected := w.selectInputsLocked(amount, fee)
	if selected == nil {
		w.mu.Unlock()
		return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: no combination of %d known notes covers amount %d + fee %d", zk.NumInputs, amount, fee)
	}
	var inSum uint64
	inSecrets := make([]zk.NoteSecret, len(selected))
	inProofs := make([]zk.Proof, len(selected))
	for i, n := range selected {
		inSum += n.secret.Value
		inSecrets[i] = n.secret
		proof, err := w.tree.Prove(n.index)
		if err != nil {
			w.mu.Unlock()
			return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: build merkle proof for a known note: %w", err)
		}
		inProofs[i] = proof
	}
	// Spend commits immediately, before any fallible work below — a
	// wallet that fails to prove or sign should not silently keep
	// offering already-selected notes to a concurrent call either.
	for c := range w.notes {
		for _, n := range selected {
			if w.notes[c] == n {
				delete(w.notes, c)
			}
		}
	}
	root := inProofs[0].Root
	w.mu.Unlock()

	change := inSum - amount - fee
	changeSecret, err := freshNoteSecret(change)
	if err != nil {
		return types.ShieldedTx{}, err
	}
	paymentSecret, err := freshNoteSecret(amount)
	if err != nil {
		return types.ShieldedTx{}, err
	}
	outSecrets := []zk.NoteSecret{paymentSecret, changeSecret}

	input := zk.TransferInput{
		MerkleRoot: root,
		Fee:        fee,
		InSecrets:  inSecrets,
		InProofs:   inProofs,
		OutSecrets: outSecrets,
	}
	zproof, err := sys.Prove(input)
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: prove transfer: %w", err)
	}
	proofBytes, err := zk.ProofToBytes(zproof)
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: serialize proof: %w", err)
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

	memo, err := EncryptMemos([]*ecdh.PublicKey{receiverPub, w.shieldedPub}, outSecrets)
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: encrypt output memos: %w", err)
	}

	t := types.ShieldedTx{
		Nullifier:            txPub.Nullifiers[0],
		Commitments:          txPub.OutCommits,
		Proof:                proofBytes,
		FeeCommit:            types.SumHash([]byte("shieldedwallet-fee"), txPub.MerkleRoot[:], txPub.Nullifiers[0][:]),
		Memo:                 memo,
		Kind:                 types.TxTransfer,
		TransferPublicInputs: txPub,
	}
	t.TxID = types.ComputeTxID(t.Proof, t.Commitments, t.Nullifier)
	sig, err := crypto.DilithiumSign(w.sk, t.TxID[:])
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("shieldedwallet: sign: %w", err)
	}
	t.Sig = types.DilithiumSig(sig)
	t.SignerPubKey = []byte(w.pk)

	return t, nil
}

// selectInputsLocked picks zk.NumInputs known notes covering amount+fee,
// preferring the fewest/largest — callers must hold w.mu.
func (w *Wallet) selectInputsLocked(amount, fee uint64) []*ownedNote {
	need := amount + fee
	all := make([]*ownedNote, 0, len(w.notes))
	for _, n := range w.notes {
		all = append(all, n)
	}
	// Simple O(n^2) largest-first selection — this build's TreeSize (16
	// leaves total, ever) makes n trivially small; clarity over
	// micro-optimizing a search space that can never be large.
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].secret.Value > all[i].secret.Value {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if len(all) < zk.NumInputs {
		return nil
	}
	// This build's circuit always spends exactly NumInputs notes (2) —
	// take the two largest and require their sum to cover amount+fee.
	// (With NumInputs fixed at 2, there is no "try 3 notes instead" path
	// to fall back to.)
	chosen := all[:zk.NumInputs]
	var sum uint64
	for _, n := range chosen {
		sum += n.secret.Value
	}
	if sum < need {
		return nil
	}
	return chosen
}

func freshNoteSecret(value uint64) (zk.NoteSecret, error) {
	sk, err := zk.NewSpendKey()
	if err != nil {
		return zk.NoteSecret{}, fmt.Errorf("shieldedwallet: generate spend key: %w", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		return zk.NoteSecret{}, fmt.Errorf("shieldedwallet: generate rho: %w", err)
	}
	return zk.NoteSecret{Value: value, OwnerSK: sk, Rho: rho}, nil
}
