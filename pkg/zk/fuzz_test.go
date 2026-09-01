package zk_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// FuzzProofFromBytesNeverPanics targets the real, attacker-controlled
// boundary spec 23's own risk register names as a Year-1 mitigation
// target: "gnark / circuit bugs... fuzz of the prover/verifier pair."
// Proof bytes arrive over the real network as part of an untrusted
// types.ShieldedTx.Proof (or MintProof/StakeProof/UnstakeProof) field —
// ProofFromBytes is the exact point where those raw, adversarial bytes
// first get deserialized, before any real cryptographic check ever
// runs. A crash here would be a real remote denial-of-service against
// every validator that receives the malformed transaction, not merely
// a rejected proof; ProofFromBytes returning a clean error for anything
// that isn't a real, well-formed proof is the actual correctness bar,
// exactly like PublicWitness's own doc.
func FuzzProofFromBytesNeverPanics(f *testing.F) {
	sys, err := zk.Setup()
	if err != nil {
		f.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(f)
	proof, err := sys.Prove(input)
	if err != nil {
		f.Fatalf("prove: %v", err)
	}
	realBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		f.Fatalf("serialize real proof: %v", err)
	}

	f.Add(realBytes)
	f.Add([]byte(nil))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add(realBytes[:len(realBytes)/2])                             // a real proof, truncated
	f.Add(append(append([]byte{}, realBytes...), 0x00, 0x01, 0x02)) // a real proof, with trailing garbage

	f.Fuzz(func(t *testing.T, data []byte) {
		// Deliberately not asserting on the error itself — only that
		// deserializing arbitrary bytes as a proof never panics. Go's own
		// fuzzing engine already treats any panic during this call as a
		// failure; a returned error (the overwhelmingly likely outcome
		// for random bytes) is the correct, safe behavior this proves.
		_, _ = zk.ProofFromBytes(data)
	})
}

// FuzzVerifyPublicProofBytesNeverPanics extends the boundary above
// through the real, full verification path pkg/tx's Stage 1 actually
// calls (VerifyPublicProofBytes) — deserialization, public-witness
// construction, and the real Groth16 verifier itself, against a real,
// otherwise-valid set of public inputs. A malformed proof must fail
// verification cleanly no matter how it's malformed, never crash the
// validator processing it.
func FuzzVerifyPublicProofBytesNeverPanics(f *testing.F) {
	sys, err := zk.Setup()
	if err != nil {
		f.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(f)
	proof, err := sys.Prove(input)
	if err != nil {
		f.Fatalf("prove: %v", err)
	}
	realBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		f.Fatalf("serialize real proof: %v", err)
	}
	pub := input.Public()

	f.Add(realBytes)
	f.Add([]byte(nil))
	f.Add([]byte{0x00})
	f.Add(realBytes[:len(realBytes)/2])

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = sys.VerifyPublicProofBytes(data, pub)
	})
}
