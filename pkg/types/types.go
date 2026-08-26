// Package types defines the canonical identifier and data-model shapes from
// spec section 4 ("Every implementer should treat the following structs as
// the canonical shapes"). Field names are camelCase per that instruction;
// semantic meaning is preserved exactly.
package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Hash is a 32-byte digest, used for TxID, Merkle roots, nullifiers, and
// commitments throughout the spec.
type Hash [32]byte

func (h Hash) String() string { return hex.EncodeToString(h[:]) }

func (h Hash) IsZero() bool { return h == Hash{} }

func (h Hash) MarshalJSON() ([]byte, error) { return marshalHexJSON(h[:]) }

func (h *Hash) UnmarshalJSON(b []byte) error { return unmarshalHexJSON(b, h[:]) }

// SumHash hashes an arbitrary sequence of byte slices with domain
// separation between fields (length-prefixed), so H(a||b) cannot be
// confused with H(a) when b is empty, or with H(ab) split differently.
func SumHash(parts ...[]byte) Hash {
	h := sha256.New()
	for _, p := range parts {
		var lenBuf [8]byte
		putUint64(lenBuf[:], uint64(len(p)))
		h.Write(lenBuf[:])
		h.Write(p)
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

func putUint64(b []byte, v uint64) {
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (8 * i))
	}
}

// Address is a 32-byte shielded account identifier (spec 4.1). Public
// explorers show only a truncated hash of it.
type Address [32]byte

func (a Address) String() string { return hex.EncodeToString(a[:]) }

func (a Address) MarshalJSON() ([]byte, error) { return marshalHexJSON(a[:]) }

func (a *Address) UnmarshalJSON(b []byte) error { return unmarshalHexJSON(b, a[:]) }

// Truncated returns the explorer-safe display form: a short hash prefix,
// never the raw address bytes (spec 8.1: "Explorers ... do not display
// sender, receiver ...").
func (a Address) Truncated() string {
	sum := sha256.Sum256(a[:])
	return hex.EncodeToString(sum[:6])
}

// AddressFromPubkey derives a stable Address from a public key blob.
func AddressFromPubkey(pk []byte) Address {
	sum := sha256.Sum256(pk)
	var a Address
	copy(a[:], sum[:])
	return a
}

// NFTID is a soulbound validator/department token id (spec 4.1, 4.5).
type NFTID [32]byte

func (n NFTID) String() string { return hex.EncodeToString(n[:]) }

func (n NFTID) MarshalJSON() ([]byte, error) { return marshalHexJSON(n[:]) }

func (n *NFTID) UnmarshalJSON(b []byte) error { return unmarshalHexJSON(b, n[:]) }

// TxID is Hash(proof || commitments || nullifier) — spec 4.1.
type TxID = Hash

// AssetID identifies an asset. SFG at launch; other assets only inside Bank
// custody (spec 4.4).
type AssetID string

const (
	AssetSFG AssetID = "SFG"
	AssetBTC AssetID = "BTC"
	AssetETH AssetID = "ETH"
)

// BlockHeight — uint64, genesis = 0 (spec 4.1).
type BlockHeight = uint64

// EpochNumber — uint64, genesis epoch = 0 (spec 4.1).
type EpochNumber = uint64

// ID is a generic string identifier used by container ids, proposal ids,
// and hold ids where the spec leaves the concrete representation open.
type ID string

func (id ID) Hash() Hash { return SumHash([]byte(id)) }

func (id ID) String() string { return string(id) }

func marshalHexJSON(b []byte) ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(b) + `"`), nil
}

func unmarshalHexJSON(b []byte, out []byte) error {
	s := string(b)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("types: invalid JSON hex string %q", s)
	}
	decoded, err := hex.DecodeString(s[1 : len(s)-1])
	if err != nil {
		return err
	}
	if len(decoded) != len(out) {
		return fmt.Errorf("types: expected %d bytes, got %d", len(out), len(decoded))
	}
	copy(out, decoded)
	return nil
}

// ParseHash decodes a hex string into a Hash, validating length.
func ParseHash(s string) (Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return Hash{}, err
	}
	if len(b) != 32 {
		return Hash{}, fmt.Errorf("types: hash must be 32 bytes, got %d", len(b))
	}
	var h Hash
	copy(h[:], b)
	return h, nil
}
