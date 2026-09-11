package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

type generateRequest struct {
	Passphrase string `json:"passphrase"`
}

type generateResponse struct {
	Address string `json:"address"`
	// KeystoreJSON is the exact real pkg/walletkey.Keystore.Save format —
	// app.js hands this back to the visitor as a download, and it's what
	// every later step (attest, mint) expects to be re-uploaded.
	KeystoreJSON string `json:"keystore_json"`
	// NodeIdentityHex is the SAME real Dilithium keypair, re-encoded in
	// cmd/node's own -key-file format (see cmd/node/main.go's
	// writeIdentityFile/decodeIdentity: 4-byte big-endian pubkey length,
	// pubkey, privkey). The real spec-4.5/10.1/243 admission gate
	// (pkg/validator's handleMessage) checks NFT ownership against
	// types.AddressFromPubkey of whatever key a validator heartbeats
	// with — so a minted NFT is only useful to a running node if that
	// node's own identity is this exact keypair. Handing back both
	// encodings of one identity here closes a real gap: nothing else in
	// this codebase bridges the wallet keystore format to the node
	// identity format, so an operator would otherwise have to mint for
	// one identity and separately, manually get a matching -key-file in
	// place for their node.
	NodeIdentityHex string `json:"node_identity_hex"`
}

// handleGenerate creates a brand-new real identity — a real Dilithium +
// X25519 keypair (pkg/walletkey.Generate, the exact function 'wallet
// identity' itself is built on) — and returns it in both formats a
// visitor needs: an encrypted keystore file to keep, and a node identity
// file to point a validator's -key-file at, so the address that ends up
// owning the minted NFT is the same address the node will heartbeat as.
// The unsealed private key only ever exists in this handler's memory,
// for the duration of this one request, and is never written to this
// server's disk unencrypted.
func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxUploadBytes)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	if len(req.Passphrase) < 8 {
		writeJSONError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	ks, err := walletkey.Generate(req.Passphrase)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("generate identity: %v", err))
		return
	}
	pk, sk, err := ks.Unlock(req.Passphrase)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("unlock freshly generated identity: %v", err))
		return
	}
	keystoreBytes, err := marshalKeystore(ks)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("encode keystore: %v", err))
		return
	}

	writeJSON(w, generateResponse{
		Address:         types.AddressFromPubkey(pk).String(),
		KeystoreJSON:    string(keystoreBytes),
		NodeIdentityHex: hex.EncodeToString(encodeNodeIdentity(pk, sk)),
	})
}

// handleIdentity reports the address and public key of an already-existing
// keystore upload — no passphrase needed, mirroring 'wallet identity'
// itself: PublicKey() is available on a locked Keystore (see pkg/
// walletkey.Keystore.PublicKey's own doc — it was never secret).
func (s *server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	path, cleanup, err := saveUploadedFile(r, "keystore")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	ks, err := walletkey.Load(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("load keystore: %v", err))
		return
	}
	writeJSON(w, map[string]string{
		"address":    types.AddressFromPubkey(ks.PublicKey()).String(),
		"public_key": hex.EncodeToString(ks.PublicKey()),
	})
}

// marshalKeystore returns ks's real on-disk JSON encoding without
// permanently writing it anywhere — Keystore.Save only writes to a path,
// so this uses one private, immediately-removed temp file as the
// round-trip, rather than reimplementing its marshaling here a second
// time.
func marshalKeystore(ks *walletkey.Keystore) ([]byte, error) {
	f, err := os.CreateTemp("", "mintpage-keystore-*.json")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	_ = f.Close()
	defer os.Remove(path)
	if err := ks.Save(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// encodeNodeIdentity mirrors cmd/node/main.go's writeIdentityFile format
// exactly ([4-byte big-endian pubkey length][pubkey][privkey]) — kept as
// a small, duplicated helper rather than an exported function shared
// across binaries, the same per-binary-helper convention cmd/wallet's own
// network.go already documents (see its waitForAddrFile).
func encodeNodeIdentity(pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey) []byte {
	buf := make([]byte, 4+len(pk)+len(sk))
	buf[0] = byte(len(pk) >> 24)
	buf[1] = byte(len(pk) >> 16)
	buf[2] = byte(len(pk) >> 8)
	buf[3] = byte(len(pk))
	copy(buf[4:], pk)
	copy(buf[4+len(pk):], sk)
	return buf
}
