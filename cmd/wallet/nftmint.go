package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

// runPoHAttest is the attestor-operator side of the real proof-of-humanity
// protocol (spec 10.1): the actual CAPTCHA/challenge a person passes is
// explicitly out of this L1 core's scope (see pkg/nft's own doc) — this
// command is what a trusted attestor runs, after doing that verification
// by whatever real means their own service uses, to sign a real,
// cryptographically binding claim a wallet can then submit as Kind
// NFTMint. It never itself decides whether someone is human; it only
// produces the real signature over that decision.
func runPoHAttest(args []string) error {
	fs := flag.NewFlagSet("poh-attest", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "the ATTESTOR's own keystore — its Dilithium key signs the attestation")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	ownerHex := fs.String("owner", "", "the requesting wallet's real address, hex — see 'wallet identity's own \"address:\" line (required)")
	nonce := fs.Uint64("nonce", 0, "the exact mint attempt nonce the requester will submit (required, must match)")
	issuedAtMs := fs.Int64("issued-at-ms", 0, "attestation issuance time, unix ms (default: now)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ownerHex == "" {
		return fmt.Errorf("-owner is required")
	}
	ownerBytes, err := hex.DecodeString(*ownerHex)
	if err != nil {
		return fmt.Errorf("-owner: %w", err)
	}
	if len(ownerBytes) != len(types.Address{}) {
		return fmt.Errorf("-owner must be %d bytes, got %d", len(types.Address{}), len(ownerBytes))
	}
	var owner types.Address
	copy(owner[:], ownerBytes)

	if *issuedAtMs == 0 {
		*issuedAtMs = time.Now().UnixMilli()
	}

	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}
	passphrase, err := readExistingPassphrase(*fromStdin)
	if err != nil {
		return err
	}
	attestorPK, attestorSK, err := ks.Unlock(passphrase)
	if err != nil {
		return err
	}

	att, err := nft.SignPoHAttestation(attestorPK, attestorSK, owner, *nonce, *issuedAtMs)
	if err != nil {
		return fmt.Errorf("sign attestation: %w", err)
	}

	fmt.Println("real, signed proof-of-humanity attestation — pass these to 'wallet nft-mint':")
	fmt.Printf("  -nonce %d\n", att.Nonce)
	fmt.Printf("  -attestation-issued-at-ms %d\n", att.IssuedAtMs)
	fmt.Printf("  -attestor-pubkey %s\n", hex.EncodeToString(att.Attestor))
	fmt.Printf("  -attestation-sig %s\n", hex.EncodeToString(att.Sig))
	fmt.Printf("valid for %s from issuance\n", nft.PoHAttestationTTL)
	return nil
}

func runNFTMint(args []string) error {
	fs := flag.NewFlagSet("nft-mint", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	nonce := fs.Uint64("nonce", 0, "the exact mint attempt nonce the attestation was signed for (required, must match)")
	issuedAtMs := fs.Int64("attestation-issued-at-ms", 0, "the attestation's real issuance time, unix ms — from 'wallet poh-attest' (required)")
	attestorHex := fs.String("attestor-pubkey", "", "the attestor's real Dilithium public key, hex — from 'wallet poh-attest' (required)")
	attestationSigHex := fs.String("attestation-sig", "", "the real attestation signature, hex — from 'wallet poh-attest' (required)")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *issuedAtMs == 0 || *attestorHex == "" || *attestationSigHex == "" {
		return fmt.Errorf("-attestation-issued-at-ms, -attestor-pubkey, and -attestation-sig are all required (see 'wallet poh-attest')")
	}
	attestor, err := hex.DecodeString(*attestorHex)
	if err != nil {
		return fmt.Errorf("-attestor-pubkey: %w", err)
	}
	attestationSig, err := hex.DecodeString(*attestationSigHex)
	if err != nil {
		return fmt.Errorf("-attestation-sig: %w", err)
	}

	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}
	passphrase, err := readExistingPassphrase(*fromStdin)
	if err != nil {
		return err
	}
	pk, sk, err := ks.Unlock(passphrase)
	if err != nil {
		return err
	}
	b := txbuilder.New(pk, sk)

	txn, err := b.NFTMint(*nonce, *issuedAtMs, crypto.DilithiumPublicKey(attestor), crypto.DilithiumSignature(attestationSig))
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}
