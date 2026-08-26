package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptionKey is a symmetric key for the "encryption wrapper in front" of
// Badger (spec 3.3 state store row) and for note/memo confidentiality.
type EncryptionKey [chacha20poly1305.KeySize]byte

// Encrypt seals plaintext under key, returning nonce||ciphertext||tag. Used
// for encrypted note blobs (spec 4.4), encrypted memos (spec 4.2 Memo
// field), and the Badger value-encryption wrapper (spec 7).
func Encrypt(key EncryptionKey, plaintext, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: aead init: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, additionalData)
	return append(nonce, sealed...), nil
}

// Decrypt opens a blob produced by Encrypt.
func Decrypt(key EncryptionKey, blob, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: aead init: %w", err)
	}
	if len(blob) < aead.NonceSize() {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := blob[:aead.NonceSize()], blob[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, ct, additionalData)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return pt, nil
}
