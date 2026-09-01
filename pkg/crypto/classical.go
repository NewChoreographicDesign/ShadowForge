// Real ed25519 support for spec 8.5's dual-sign migration path: "Dual-sign
// (Dilithium + ed25519) is allowed only as a migration aid and must be
// scheduled for removal by governance." Dilithium (see dilithium.go) is
// unconditionally required everywhere this codebase signs anything — this
// file adds the OPTIONAL classical co-signature spec 8.5 describes
// alongside it, never in place of it, using Go's own standard-library
// ed25519 implementation (crypto/ed25519) rather than a third-party one,
// since ed25519 is a stable, well-reviewed classical scheme with no post-
// quantum ambition of its own — there is nothing to gain from a second
// implementation of it the way there is for Dilithium.
package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

// ClassicalPublicKey, ClassicalPrivateKey, and ClassicalSignature are
// ed25519's own fixed-size byte blobs, named and shaped to mirror
// DilithiumPublicKey/DilithiumPrivateKey/DilithiumSignature exactly —
// the same "detached signature over TxID" scheme, a different algorithm.
type ClassicalPublicKey []byte
type ClassicalPrivateKey []byte
type ClassicalSignature []byte

// GenerateClassicalKey creates a fresh ed25519 keypair for the dual-sign
// migration path — real defense-in-depth during the window spec 8.5
// describes, never a replacement for the Dilithium key every signer
// still unconditionally needs.
func GenerateClassicalKey() (ClassicalPublicKey, ClassicalPrivateKey, error) {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: ed25519 keygen: %w", err)
	}
	return ClassicalPublicKey(pk), ClassicalPrivateKey(sk), nil
}

// ClassicalSign signs msg with sk.
func ClassicalSign(sk ClassicalPrivateKey, msg []byte) (ClassicalSignature, error) {
	if len(sk) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("crypto: classical private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(sk))
	}
	return ClassicalSignature(ed25519.Sign(ed25519.PrivateKey(sk), msg)), nil
}

// ClassicalVerify checks sig over msg against pk.
func ClassicalVerify(pk ClassicalPublicKey, msg []byte, sig ClassicalSignature) (bool, error) {
	if len(pk) != ed25519.PublicKeySize {
		return false, fmt.Errorf("crypto: classical public key must be %d bytes, got %d", ed25519.PublicKeySize, len(pk))
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("crypto: classical signature must be %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}
	return ed25519.Verify(ed25519.PublicKey(pk), msg, sig), nil
}

func (k ClassicalPublicKey) String() string  { return hex.EncodeToString(k) }
func (s ClassicalSignature) String() string  { return hex.EncodeToString(s) }
func (k ClassicalPrivateKey) String() string { return "<redacted classical private key>" }
