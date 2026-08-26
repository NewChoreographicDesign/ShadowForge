// Package crypto wraps the post-quantum and hashing primitives spec section
// 8.5 requires: "Dilithium (or an equivalent NIST PQC signature) is the
// default signature scheme for stages and proofs." We use CRYSTALS-Dilithium
// mode3 (NIST security category 3) via cloudflare/circl, the community Go
// implementation named in the canonical stack (spec 3.3).
package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cloudflare/circl/sign/dilithium/mode3"
)

// DilithiumPublicKey, DilithiumPrivateKey, and DilithiumSignature are byte
// blobs sized to mode3's fixed key/signature lengths.
type DilithiumPublicKey []byte
type DilithiumPrivateKey []byte
type DilithiumSignature []byte

// GenerateDilithiumKey creates a fresh wallet/stage/validator signing
// keypair (spec 8.5: "Dilithium signs: wallet authorizations, stage votes,
// block proposals, container sync aggregates").
func GenerateDilithiumKey() (DilithiumPublicKey, DilithiumPrivateKey, error) {
	pk, sk, err := mode3.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: dilithium keygen: %w", err)
	}
	return DilithiumPublicKey(pk.Bytes()), DilithiumPrivateKey(sk.Bytes()), nil
}

// DilithiumSign signs msg with sk.
func DilithiumSign(sk DilithiumPrivateKey, msg []byte) (DilithiumSignature, error) {
	var skBuf [mode3.PrivateKeySize]byte
	if len(sk) != mode3.PrivateKeySize {
		return nil, fmt.Errorf("crypto: private key must be %d bytes, got %d", mode3.PrivateKeySize, len(sk))
	}
	copy(skBuf[:], sk)
	var priv mode3.PrivateKey
	priv.Unpack(&skBuf)

	sig := make([]byte, mode3.SignatureSize)
	mode3.SignTo(&priv, msg, sig)
	return DilithiumSignature(sig), nil
}

// DilithiumVerify checks sig over msg against pk.
func DilithiumVerify(pk DilithiumPublicKey, msg []byte, sig DilithiumSignature) (bool, error) {
	if len(pk) != mode3.PublicKeySize {
		return false, fmt.Errorf("crypto: public key must be %d bytes, got %d", mode3.PublicKeySize, len(pk))
	}
	if len(sig) != mode3.SignatureSize {
		return false, errors.New("crypto: signature has the wrong length")
	}
	var pkBuf [mode3.PublicKeySize]byte
	copy(pkBuf[:], pk)
	var pub mode3.PublicKey
	pub.Unpack(&pkBuf)
	return mode3.Verify(&pub, msg, sig), nil
}

func (k DilithiumPublicKey) String() string  { return hex.EncodeToString(k) }
func (s DilithiumSignature) String() string  { return hex.EncodeToString(s) }
func (k DilithiumPrivateKey) String() string { return "<redacted dilithium private key>" }
