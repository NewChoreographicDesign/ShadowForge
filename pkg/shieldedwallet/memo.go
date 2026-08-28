// Package shieldedwallet is Tier B priority #5: real client-side support
// for Kind Transfer — the one ShieldedTx kind pkg/txbuilder deliberately
// left out, because it needs a real Groth16 proof over real notes, not
// just a correctly-signed public-input struct.
//
// Real Shielded Transfer support turned out to need two genuinely
// separate pieces, both closed here:
//
//  1. Nothing in this codebase — not the implementation, not the spec —
//     ever defined how a *receiver* learns the secrets of a note sent to
//     them. types.ShieldedTx.Memo was reserved for exactly this
//     ("optional encrypted memo for receiver") but never wired. This
//     package wires it for real: every wallet identity now carries a
//     second, real X25519 key-agreement pair (pkg/walletkey) alongside
//     its Dilithium signing pair, and memo.go implements a real,
//     standard ECIES construction (ephemeral X25519 + HKDF-SHA256 +
//     ChaCha20-Poly1305) to seal a note's full opening (value, spend
//     key, rho) to a receiver's public key. The sender briefly knows the
//     spend key too, since they generated it — a real, disclosed trust
//     boundary this design accepts rather than hides, not a full
//     stealth-address scheme.
//  2. Live testing surfaced that pkg/tx's pipeline never checked a
//     Transfer proof's claimed Merkle root against anything real — see
//     pkg/zk.RootHistory's own doc for that fix. This package's Sync
//     (sync.go) is what lets a real wallet build a proof anchored to a
//     root a real node will actually recognize, by replaying the same
//     committed history every honest node does.
package shieldedwallet

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// memoHKDFInfo domain-separates this package's memo key derivation from
// any other use of HKDF/ECDH over the same X25519 keys.
var memoHKDFInfo = []byte("shadowforge-shieldedwallet-memo-v1")

// noteOpeningSize is the fixed wire size of one note's plaintext opening:
// an 8-byte big-endian value, then two 32-byte field elements (OwnerSK,
// Rho) — a fixed binary layout rather than JSON, so the plaintext's own
// length never varies with the value's magnitude and can't leak anything
// through padding differences.
const noteOpeningSize = 8 + 32 + 32

// x25519PubLen is X25519's fixed public-key size — a memo's first
// x25519PubLen bytes are always the sender's ephemeral public key.
const x25519PubLen = 32

// EncryptMemo seals secret's full opening (value, spend key, rho) to
// receiverPub, producing the bytes a real ShieldedTx.Memo field carries.
// A fresh ephemeral X25519 keypair is generated per call — the ephemeral
// public key travels in the clear as the memo's first 32 bytes (it isn't
// secret; ECDH's security comes from the ephemeral *private* key never
// being reused or revealed), so a receiver who only has their own
// long-term private key can still derive the same shared secret.
func EncryptMemo(receiverPub *ecdh.PublicKey, secret zk.NoteSecret) ([]byte, error) {
	curve := ecdh.X25519()
	ephemeral, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("shieldedwallet: generate ephemeral key: %w", err)
	}
	shared, err := ephemeral.ECDH(receiverPub)
	if err != nil {
		return nil, fmt.Errorf("shieldedwallet: ECDH: %w", err)
	}
	key, err := deriveMemoKey(shared)
	if err != nil {
		return nil, err
	}

	plain := encodeNoteOpening(secret)
	// The ephemeral public key is bound in as associated data: an
	// attacker who swapped in a different ephemeral key (the only part
	// of the memo that isn't itself authenticated by the AEAD tag over
	// the ciphertext) would break decryption rather than silently having
	// the receiver derive a different, attacker-influenced shared key
	// under a mismatched label.
	ephemeralPubBytes := ephemeral.PublicKey().Bytes()
	ciphertext, err := crypto.Encrypt(key, plain, ephemeralPubBytes)
	if err != nil {
		return nil, fmt.Errorf("shieldedwallet: seal memo: %w", err)
	}

	memo := make([]byte, 0, len(ephemeralPubBytes)+len(ciphertext))
	memo = append(memo, ephemeralPubBytes...)
	memo = append(memo, ciphertext...)
	return memo, nil
}

// DecryptMemo attempts to open memo as a note addressed to myPriv. A
// memo that isn't really addressed to this identity (wrong receiver, or
// simply not a shielded-note memo at all) fails here — this is exactly
// how a real wallet tells "mine" apart from "not mine" while scanning
// chain data it has no other way to distinguish (pkg/query never reveals
// who a Transfer's receiver is, by design).
func DecryptMemo(myPriv *ecdh.PrivateKey, memo []byte) (zk.NoteSecret, error) {
	if len(memo) < x25519PubLen {
		return zk.NoteSecret{}, errors.New("shieldedwallet: memo too short to contain an ephemeral public key")
	}
	ephemeralPubBytes := memo[:x25519PubLen]
	ciphertext := memo[x25519PubLen:]

	ephemeralPub, err := ecdh.X25519().NewPublicKey(ephemeralPubBytes)
	if err != nil {
		return zk.NoteSecret{}, fmt.Errorf("shieldedwallet: invalid ephemeral public key: %w", err)
	}
	shared, err := myPriv.ECDH(ephemeralPub)
	if err != nil {
		return zk.NoteSecret{}, fmt.Errorf("shieldedwallet: ECDH: %w", err)
	}
	key, err := deriveMemoKey(shared)
	if err != nil {
		return zk.NoteSecret{}, err
	}

	plain, err := crypto.Decrypt(key, ciphertext, ephemeralPubBytes)
	if err != nil {
		// The overwhelmingly common, expected reason to land here while
		// scanning chain data: this memo simply wasn't addressed to us.
		return zk.NoteSecret{}, errors.New("shieldedwallet: memo not addressed to this identity")
	}
	return decodeNoteOpening(plain)
}

func deriveMemoKey(sharedSecret []byte) (crypto.EncryptionKey, error) {
	var key crypto.EncryptionKey
	r := hkdf.New(sha256.New, sharedSecret, nil, memoHKDFInfo)
	if _, err := io.ReadFull(r, key[:]); err != nil {
		return crypto.EncryptionKey{}, fmt.Errorf("shieldedwallet: derive memo key: %w", err)
	}
	return key, nil
}

func encodeNoteOpening(secret zk.NoteSecret) []byte {
	buf := make([]byte, noteOpeningSize)
	binary.BigEndian.PutUint64(buf[0:8], secret.Value)
	ownerSKBytes := secret.OwnerSK.Bytes()
	rhoBytes := secret.Rho.Bytes()
	copy(buf[8:40], ownerSKBytes[:])
	copy(buf[40:72], rhoBytes[:])
	return buf
}

func decodeNoteOpening(plain []byte) (zk.NoteSecret, error) {
	if len(plain) != noteOpeningSize {
		return zk.NoteSecret{}, fmt.Errorf("shieldedwallet: decrypted note opening is %d bytes, want %d", len(plain), noteOpeningSize)
	}
	var secret zk.NoteSecret
	secret.Value = binary.BigEndian.Uint64(plain[0:8])
	secret.OwnerSK.SetBytes(plain[8:40])
	secret.Rho.SetBytes(plain[40:72])
	return secret, nil
}
