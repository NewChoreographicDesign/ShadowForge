package walletkey

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
)

// argon2Params are the Argon2id tuning knobs, stored alongside every
// keystore file rather than hard-coded at the call site — the same
// reasoning real password-hash formats (e.g. Ethereum's UTC keystore,
// bcrypt's own embedded cost byte) use: parameters can be strengthened
// for newly-created keystores later without breaking the ability to open
// one written under the old parameters.
type argon2Params struct {
	TimeCost   uint32 `json:"time_cost"`
	MemoryKiB  uint32 `json:"memory_kib"`
	Threads    uint8  `json:"threads"`
	SaltLength uint32 `json:"salt_length"`
}

// defaultArgon2Params follows current OWASP guidance for Argon2id used as
// a password-based KDF (not a fast general-purpose hash): a 64 MiB memory
// cost is deliberately expensive enough to make brute-forcing a stolen
// keystore file's passphrase costly, while still completing in well under
// a second on ordinary hardware — this runs once per unlock, not in any
// hot path.
var defaultArgon2Params = argon2Params{
	TimeCost:   3,
	MemoryKiB:  64 * 1024,
	Threads:    4,
	SaltLength: 16,
}

// deriveKey stretches passphrase into a real crypto.EncryptionKey via
// Argon2id, salted per keystore. Never called with an empty passphrase —
// callers (Generate, Unlock) reject that before this runs.
func deriveKey(passphrase string, salt []byte, p argon2Params) crypto.EncryptionKey {
	raw := argon2.IDKey([]byte(passphrase), salt, p.TimeCost, p.MemoryKiB, p.Threads, uint32(len(crypto.EncryptionKey{})))
	var key crypto.EncryptionKey
	copy(key[:], raw)
	zero(raw)
	return key
}

// newSalt generates a fresh random salt of the configured length.
func newSalt(n uint32) ([]byte, error) {
	salt := make([]byte, n)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("walletkey: generate salt: %w", err)
	}
	return salt, nil
}

// zero best-effort overwrites b with zeros before it's discarded. Go's
// garbage collector can still have moved or copied the underlying bytes
// elsewhere before this runs (a stack copy during a prior function call,
// for instance), so this narrows the window sensitive material sits in
// memory — it is not a guarantee against a determined attacker with
// memory access, and this package makes no claim that it is.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
