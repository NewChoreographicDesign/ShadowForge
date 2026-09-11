package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/txclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

type attestResponse struct {
	Nonce           uint64 `json:"nonce"`
	IssuedAtMs      int64  `json:"issued_at_ms"`
	AttestorPubkey  string `json:"attestor_pubkey"`
	AttestationSig  string `json:"attestation_sig"`
	ValidForSeconds int    `json:"valid_for_seconds"`
}

// handleAttest is the ATTESTOR's side of the real proof-of-humanity
// protocol (spec 10.1) — the same operation 'wallet poh-attest' performs,
// wrapped in this page as a convenience for an operator who has already
// decided, by their own real means, that the requester behind -owner is
// a real person applying once. It never itself makes that judgment; it
// only produces the real, cryptographically binding signature over it
// (pkg/nft.SignPoHAttestation), exactly like the CLI command does.
func (s *server) handleAttest(w http.ResponseWriter, r *http.Request) {
	path, cleanup, err := saveUploadedFile(r, "keystore")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	ownerHex := strings.TrimSpace(r.FormValue("owner"))
	nonceStr := strings.TrimSpace(r.FormValue("nonce"))
	passphrase := r.FormValue("passphrase")

	ownerBytes, err := hex.DecodeString(ownerHex)
	if err != nil || len(ownerBytes) != len(types.Address{}) {
		writeJSONError(w, http.StatusBadRequest, "owner must be the requester's 64-hex-character address")
		return
	}
	var owner types.Address
	copy(owner[:], ownerBytes)

	nonce, err := strconv.ParseUint(nonceStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "nonce must be a non-negative integer")
		return
	}

	ks, err := walletkey.Load(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("load attestor keystore: %v", err))
		return
	}
	attestorPK, attestorSK, err := ks.Unlock(passphrase)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "wrong passphrase, or corrupted attestor keystore")
		return
	}

	issuedAtMs := time.Now().UnixMilli()
	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, owner, nonce, issuedAtMs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("sign attestation: %v", err))
		return
	}
	writeJSON(w, attestResponse{
		Nonce:           att.Nonce,
		IssuedAtMs:      att.IssuedAtMs,
		AttestorPubkey:  hex.EncodeToString(att.Attestor),
		AttestationSig:  hex.EncodeToString(att.Sig),
		ValidForSeconds: int(nft.PoHAttestationTTL.Seconds()),
	})
}

type mintResponse struct {
	TxID   string  `json:"txid"`
	Status string  `json:"status"`
	Height *uint64 `json:"height,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// handleMint is the same real path 'wallet nft-mint' + submitTx already
// exercise: build a real, signed TxNFTMint (pkg/txbuilder.Builder.
// NFTMint) binding the real attestation from handleAttest (or from
// whatever attestor the requester's operator actually used), open a
// fresh real libp2p connection to a bootstrap peer, broadcast it as a
// real TxOffer, and — unless the caller asked for confirm_timeout_seconds
// 0 — wait for pkg/query to report it committed (pkg/txclient.Client.
// SubmitAndConfirm), the exact same real submit-and-confirm loop
// cmd/wallet's own submitTx uses. A transaction the real pipeline
// rejects (already minted, expired or untrusted attestation, wrong
// nonce) behaves exactly as it does for the CLI: it never commits, so
// this simply times out rather than reporting a false rejection reason
// — pkg/query's /v1/tx/{txid} has no separate "rejected" state (see its
// own doc), which is an existing, honest limitation this page inherits
// rather than papers over.
func (s *server) handleMint(w http.ResponseWriter, r *http.Request) {
	path, cleanup, err := saveUploadedFile(r, "keystore")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	passphrase := r.FormValue("passphrase")
	nonceStr := strings.TrimSpace(r.FormValue("nonce"))
	issuedAtMsStr := strings.TrimSpace(r.FormValue("issued_at_ms"))
	attestorHex := strings.TrimSpace(r.FormValue("attestor_pubkey"))
	attestationSigHex := strings.TrimSpace(r.FormValue("attestation_sig"))
	bootstrap := strings.TrimSpace(r.FormValue("bootstrap"))
	queryRaw := strings.TrimSpace(r.FormValue("query"))
	confirmSecondsStr := strings.TrimSpace(r.FormValue("confirm_timeout_seconds"))

	if nonceStr == "" || issuedAtMsStr == "" || attestorHex == "" || attestationSigHex == "" {
		writeJSONError(w, http.StatusBadRequest, "nonce, issued_at_ms, attestor_pubkey, and attestation_sig are all required (from the Attest step)")
		return
	}
	if bootstrap == "" {
		writeJSONError(w, http.StatusBadRequest, "a bootstrap peer multiaddr is required to broadcast the mint transaction to")
		return
	}
	nonce, err := strconv.ParseUint(nonceStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "nonce must be a non-negative integer")
		return
	}
	issuedAtMs, err := strconv.ParseInt(issuedAtMsStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "issued_at_ms must be an integer")
		return
	}
	attestor, err := hex.DecodeString(attestorHex)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "attestor_pubkey must be hex")
		return
	}
	attestationSig, err := hex.DecodeString(attestationSigHex)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "attestation_sig must be hex")
		return
	}
	var queryURLs []string
	for _, p := range strings.Split(queryRaw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			queryURLs = append(queryURLs, p)
		}
	}

	confirmTimeout := 30 * time.Second
	if confirmSecondsStr != "" {
		secs, err := strconv.Atoi(confirmSecondsStr)
		if err != nil || secs < 0 {
			writeJSONError(w, http.StatusBadRequest, "confirm_timeout_seconds must be a non-negative integer")
			return
		}
		confirmTimeout = time.Duration(secs) * time.Second
	}
	if confirmTimeout > s.maxConfirmTimeout {
		confirmTimeout = s.maxConfirmTimeout
	}

	ks, err := walletkey.Load(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("load keystore: %v", err))
		return
	}
	pk, sk, err := ks.Unlock(passphrase)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "wrong passphrase, or corrupted keystore")
		return
	}

	b := txbuilder.New(pk, sk)
	txn, err := b.NFTMint(nonce, issuedAtMs, crypto.DilithiumPublicKey(attestor), crypto.DilithiumSignature(attestationSig))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("build mint transaction: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), confirmTimeout+15*time.Second)
	defer cancel()

	h, err := shadownet.NewHost("/ip4/0.0.0.0/tcp/0")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("create libp2p host: %v", err))
		return
	}
	defer func() { _ = h.Close() }()

	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connectCancel()
	if err := shadownet.Connect(connectCtx, h, bootstrap); err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("connect to bootstrap peer %s: %v", bootstrap, err))
		return
	}

	node := shadownet.NewNode(h, nil, nil)
	client, err := txclient.New(txclient.Config{Net: node, QueryURLs: queryURLs})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("create tx client: %v", err))
		return
	}

	if confirmTimeout <= 0 || len(queryURLs) == 0 {
		if err := client.Submit(ctx, txn); err != nil {
			writeJSON(w, mintResponse{TxID: txn.TxID.String(), Status: "submit-failed", Error: err.Error()})
			return
		}
		writeJSON(w, mintResponse{TxID: txn.TxID.String(), Status: "submitted"})
		return
	}

	st, err := client.SubmitAndConfirm(ctx, txn, confirmTimeout)
	if err != nil {
		writeJSON(w, mintResponse{TxID: txn.TxID.String(), Status: "not-confirmed", Error: err.Error()})
		return
	}
	writeJSON(w, mintResponse{TxID: txn.TxID.String(), Status: st.State, Height: st.Height})
}
