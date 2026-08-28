package zk

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	bn254mimc "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
)

// FieldElement is a BN254 scalar-field element — the native unit the ZK
// circuit and its off-circuit witness-preparation helpers both operate on.
// This is deliberately a different type from types.Hash (a general-purpose
// 32-byte SHA256 digest used elsewhere in the node): the circuit needs an
// R1CS-efficient hash (MiMC) over its native field, while block/state
// hashing has no such constraint. See the pkg/zk package doc for how the
// two are bridged.
type FieldElement = fr.Element

func newHasher() bn254mimc.FieldHasher {
	h := bn254mimc.NewMiMC()
	fh, ok := h.(bn254mimc.FieldHasher)
	if !ok {
		panic("zk: mimc.NewMiMC did not return a FieldHasher")
	}
	return fh
}

// mimcHash is the off-circuit counterpart of the in-circuit
// `h.Write(...); h.Sum()` calls in circuit.go — it must compute exactly the
// same function so a real Prove() call succeeds.
func mimcHash(elems ...FieldElement) FieldElement {
	h := newHasher()
	h.Reset()
	return h.SumElements(elems)
}

// RandomFieldElement draws a uniform element of the BN254 scalar field, for
// generating rho and spend keys.
func RandomFieldElement() (FieldElement, error) {
	var e FieldElement
	if _, err := e.SetRandom(); err != nil {
		return FieldElement{}, fmt.Errorf("zk: random field element: %w", err)
	}
	return e, nil
}

// ValueElement converts a note's plain uint64 value into a field element.
func ValueElement(v uint64) FieldElement {
	var e FieldElement
	e.SetUint64(v)
	return e
}

// NoteSecret is the private opening of one note (spec 4.4's Note, plus the
// spend key needed to derive OwnerPK and the nullifier).
type NoteSecret struct {
	Value   uint64
	OwnerSK FieldElement
	Rho     FieldElement
}

// OwnerPK derives the note's public spend-key binding: ownerPK = MiMC(ownerSK).
func (n NoteSecret) OwnerPK() FieldElement {
	return mimcHash(n.OwnerSK)
}

// Commitment computes commitment = MiMC(value, ownerPK, rho), matching the
// in-circuit computation exactly.
func (n NoteSecret) Commitment() FieldElement {
	return mimcHash(ValueElement(n.Value), n.OwnerPK(), n.Rho)
}

// Nullifier computes nullifier = MiMC(rho, ownerSK), matching the in-circuit
// computation exactly.
func (n NoteSecret) Nullifier() FieldElement {
	return mimcHash(n.Rho, n.OwnerSK)
}

// NewSpendKey draws a fresh random spend key.
func NewSpendKey() (FieldElement, error) { return RandomFieldElement() }

// NewRho draws a fresh random nullifier seed.
func NewRho() (FieldElement, error) { return RandomFieldElement() }

// StakeSecret is the private opening of one locked spec-17.4 staked-yield
// position (StakeCircuit) — NoteSecret's shape (a value, a spend key, and
// a nullifier seed) plus StartEpoch, the one addition StakeCircuit's own
// doc explains: because a later Unstake proof (UnstakeCircuit) must never
// reveal which position it opens, StartEpoch has to be provable purely
// from the hidden leaf's own preimage rather than looked up by leaf
// identity in some separate record, so it is baked directly into the
// commitment here instead of tracked alongside it.
type StakeSecret struct {
	Principal  uint64
	StartEpoch uint64
	OwnerSK    FieldElement
	Rho        FieldElement
}

// OwnerPK derives the position's public spend-key binding: ownerPK =
// MiMC(ownerSK) — identical formula to NoteSecret.OwnerPK().
func (s StakeSecret) OwnerPK() FieldElement {
	return mimcHash(s.OwnerSK)
}

// Commitment computes commitment = MiMC(principal, startEpoch, ownerPK,
// rho), matching StakeCircuit's in-circuit computation exactly.
func (s StakeSecret) Commitment() FieldElement {
	return mimcHash(ValueElement(s.Principal), ValueElement(s.StartEpoch), s.OwnerPK(), s.Rho)
}

// Nullifier computes nullifier = MiMC(rho, ownerSK) — the identical
// formula NoteSecret.Nullifier() already uses, deliberately reused rather
// than a second one invented for this second commitment scheme (see
// UnstakeCircuit's own doc on why that reuse is safe): pkg/tx's pipeline
// checks it against the same nullifier-spent set an ordinary note's own
// nullifier already shares.
func (s StakeSecret) Nullifier() FieldElement {
	return mimcHash(s.Rho, s.OwnerSK)
}

// FieldElementFromBigInt / ToBigInt are small convenience wrappers used by
// callers that store values as *big.Int (e.g. gnark witness assignment).
func FieldElementFromBigInt(v *big.Int) FieldElement {
	var e FieldElement
	e.SetBigInt(v)
	return e
}
