package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/govwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// loadBuilder unlocks path's Dilithium identity (real terminal prompt, or
// -passphrase-stdin) and wraps it in a real txbuilder.Builder.
func loadBuilder(path string, fromStdin bool) (*txbuilder.Builder, error) {
	ks, err := walletkey.Load(path)
	if err != nil {
		return nil, err
	}
	passphrase, err := readExistingPassphrase(fromStdin)
	if err != nil {
		return nil, err
	}
	pk, sk, err := ks.Unlock(passphrase)
	if err != nil {
		return nil, err
	}
	return txbuilder.New(pk, sk), nil
}

// buildVoteEligibility syncs a real pkg/govwallet.Wallet for the identity
// at path (the one that minted the real NFT — never the throwaway
// vote-signing key runVote/runVoteReveal generate below) against a live
// node, and uses it to build a real anonymous VoteEligibilityProof for
// proposalID. Shared by runVote and runVoteReveal: both need this
// exact same real Sync-then-prove flow, differing only in what tx kind
// wraps the resulting proof.
func buildVoteEligibility(ctx context.Context, keystorePath string, fromStdin bool, queryURL, eligibilityParams string, proposalID types.ID) (types.VoteEligibilityProof, error) {
	if queryURL == "" {
		return types.VoteEligibilityProof{}, fmt.Errorf("-query is required: proving real anonymous eligibility needs a live node to sync the eligibility-commitment tree from")
	}
	ks, err := walletkey.Load(keystorePath)
	if err != nil {
		return types.VoteEligibilityProof{}, err
	}
	passphrase, err := readExistingPassphrase(fromStdin)
	if err != nil {
		return types.VoteEligibilityProof{}, err
	}
	_, sk, err := ks.Unlock(passphrase)
	if err != nil {
		return types.VoteEligibilityProof{}, err
	}

	gw, err := govwallet.New(sk, govwallet.Config{QueryBase: queryURL})
	if err != nil {
		return types.VoteEligibilityProof{}, err
	}
	if err := gw.Sync(ctx); err != nil {
		return types.VoteEligibilityProof{}, fmt.Errorf("sync eligibility tree: %w", err)
	}
	if !gw.Eligible() {
		return types.VoteEligibilityProof{}, fmt.Errorf("this wallet's real, minted NFT was not found in any synced Kind NFTMint — run 'wallet nft-mint' first, then retry")
	}
	sys, err := loadEligibilitySystem(eligibilityParams)
	if err != nil {
		return types.VoteEligibilityProof{}, err
	}
	return gw.BuildEligibilityProof(sys, proposalID)
}

// throwawayVoteSigner generates a fresh, unlinked Dilithium keypair to
// sign the Vote/VoteReveal transaction envelope itself — deliberately not
// the same identity that minted the NFT (loaded separately in
// buildVoteEligibility above). The real anonymous eligibility proof, not
// the tx signature, is what proves eligibility now; signing with the
// NFT-holder's own long-lived key here would defeat the whole point by
// re-attaching a public identity to every ballot (see types.
// VotePublicInputs' own doc).
func throwawayVoteSigner() (*txbuilder.Builder, error) {
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		return nil, fmt.Errorf("generate throwaway vote-signing key: %w", err)
	}
	return txbuilder.New(pk, sk), nil
}

func runVote(args []string) error {
	fs := flag.NewFlagSet("vote", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock — the identity that minted the real NFT this vote proves eligibility from")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	proposal := fs.String("proposal", "", "proposal id (required)")
	approve := fs.Bool("approve", false, "cast an approve ballot (default: reject)")
	paramKey := fs.String("param-key", "", "optional governance.Params field this proposal changes (only matters on the first Vote for this proposal id)")
	newValue := fs.String("new-value", "", "new value for -param-key")
	eligibilityParams := fs.String("eligibility-zk-params", "", "path to the real, shared Groth16 params file for anonymous voter eligibility (see 'wallet eligibility-zk-setup' and cmd/node's -eligibility-zk-params) — required")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposal == "" {
		return fmt.Errorf("-proposal is required")
	}
	ctx := context.Background()
	queryURL, err := nf.firstQueryURL()
	if err != nil {
		return err
	}
	eligibility, err := buildVoteEligibility(ctx, *path, *fromStdin, queryURL, *eligibilityParams, types.ID(*proposal))
	if err != nil {
		return err
	}
	b, err := throwawayVoteSigner()
	if err != nil {
		return err
	}
	txn, err := b.Vote(types.ID(*proposal), *approve, *paramKey, *newValue, eligibility)
	if err != nil {
		return err
	}
	return submitTx(ctx, &nf, txn)
}

func runVoteReveal(args []string) error {
	fs := flag.NewFlagSet("vote-reveal", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock — the identity that minted the real NFT this vote proves eligibility from")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	proposal := fs.String("proposal", "", "proposal id (required) — must match an earlier vote")
	approve := fs.Bool("approve", false, "must be the exact same value passed to the matching vote")
	eligibilityParams := fs.String("eligibility-zk-params", "", "path to the real, shared Groth16 params file for anonymous voter eligibility (see 'wallet eligibility-zk-setup' and cmd/node's -eligibility-zk-params) — required")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposal == "" {
		return fmt.Errorf("-proposal is required")
	}
	ctx := context.Background()
	queryURL, err := nf.firstQueryURL()
	if err != nil {
		return err
	}
	// Rebuilding a fresh eligibility proof here (rather than reusing
	// Vote's) still reproduces the identical Nullifier — it's
	// deterministic in (VoterSK, ProposalID) — which is what lets this
	// reveal open the exact ballot the matching Vote committed.
	eligibility, err := buildVoteEligibility(ctx, *path, *fromStdin, queryURL, *eligibilityParams, types.ID(*proposal))
	if err != nil {
		return err
	}
	b, err := throwawayVoteSigner()
	if err != nil {
		return err
	}
	txn, err := b.VoteReveal(types.ID(*proposal), *approve, eligibility)
	if err != nil {
		return err
	}
	return submitTx(ctx, &nf, txn)
}

// runProposeMint drives the real spec-17.4 epoch-mint proposer path
// (txbuilder.Builder.ProposeMint): a real Groth16 proof binds the
// requested amount to a real output note commitment, both submitted
// inside an ordinary sealed-ballot TxVote. Like vote/vote-reveal, it
// signs the transaction envelope with a fresh, throwaway key.
func runProposeMint(args []string) error {
	fs := flag.NewFlagSet("propose-mint", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock — the identity that minted the real NFT this proposal proves eligibility from")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	proposal := fs.String("proposal", "", "proposal id (required)")
	approve := fs.Bool("approve", true, "cast an approve ballot for this proposal's own first vote (default: true — proposing without approving is unusual but allowed)")
	amount := fs.Uint64("amount", 0, "SFG amount requested (required, > 0) — the real note this mint creates, once passed, holds amount minus the real Vault fee (see types.MintNetAmount)")
	eligibilityParams := fs.String("eligibility-zk-params", "", "path to the real, shared Groth16 params file for anonymous voter eligibility (see 'wallet eligibility-zk-setup' and cmd/node's -eligibility-zk-params) — required")
	mintParams := fs.String("mint-zk-params", "", "path to the real, shared Groth16 params file for the epoch-mint circuit (see 'wallet mint-zk-setup' and cmd/node's -mint-zk-params) — required")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposal == "" {
		return fmt.Errorf("-proposal is required")
	}
	if *amount == 0 {
		return fmt.Errorf("-amount must be greater than 0")
	}
	ctx := context.Background()
	queryURL, err := nf.firstQueryURL()
	if err != nil {
		return err
	}
	eligibility, err := buildVoteEligibility(ctx, *path, *fromStdin, queryURL, *eligibilityParams, types.ID(*proposal))
	if err != nil {
		return err
	}
	mintSys, err := loadMintSystem(*mintParams)
	if err != nil {
		return err
	}
	b, err := throwawayVoteSigner()
	if err != nil {
		return err
	}
	txn, secret, err := b.ProposeMint(types.ID(*proposal), *approve, *amount, mintSys, eligibility)
	if err != nil {
		return err
	}
	commitBytes := zk.ToBytes32(secret.Commitment())
	ownerSKBytes := zk.ToBytes32(secret.OwnerSK)
	rhoBytes := zk.ToBytes32(secret.Rho)
	fmt.Println("real mint proposal built and proved — save this note opening, it is the ONLY way to ever spend the minted value once this proposal passes:")
	fmt.Printf("  -commitment %s\n", hex.EncodeToString(commitBytes[:]))
	fmt.Printf("  -value %d\n", secret.Value)
	fmt.Printf("  -owner-sk %s\n", hex.EncodeToString(ownerSKBytes[:]))
	fmt.Printf("  -rho %s\n", hex.EncodeToString(rhoBytes[:]))
	fmt.Println("check 'wallet proposal -id <id>' for real Passed/MintApplied status once tallied")
	return submitTx(ctx, &nf, txn)
}

func runMint(args []string) error {
	fs := flag.NewFlagSet("mint", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.Mint()
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}

func runNFTTrait(args []string) error {
	fs := flag.NewFlagSet("nft-trait", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	targetHex := fs.String("target", "", "target NFT id, hex (must already be minted)")
	key := fs.String("key", "", "trait key")
	delta := fs.Int64("delta", 0, "shielded delta to commit for this trait")
	saltHex := fs.String("salt", "", "salt, hex (default: fresh random bytes — must be kept to later prove what the commitment opens to)")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *targetHex == "" || *key == "" {
		return fmt.Errorf("-target and -key are required")
	}
	target, err := parseNFTID(*targetHex)
	if err != nil {
		return fmt.Errorf("-target: %w", err)
	}
	var salt []byte
	if *saltHex != "" {
		salt, err = hex.DecodeString(*saltHex)
		if err != nil {
			return fmt.Errorf("-salt: %w", err)
		}
	} else {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return fmt.Errorf("generate random salt: %w", err)
		}
		fmt.Printf("salt (save this to later prove what the commitment opens to): %s\n", hex.EncodeToString(salt))
	}

	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.NFTTrait(target, *key, *delta, salt)
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}

// oracleQuorumFromFlags builds the same real, two-source oracle.Quorum
// (CoinGecko + Coinbase) a live validator node checks BankDeposit/
// BankWithdraw claims against — see cmd/node's identical construction.
func oracleQuorumFromFlags(maxDisagreement string) (*oracle.Quorum, error) {
	d, err := decimal.FromString(maxDisagreement)
	if err != nil {
		return nil, fmt.Errorf("-oracle-max-disagreement %q: %w", maxDisagreement, err)
	}
	return oracle.NewQuorum(d, oracle.CoinGeckoSource{}, oracle.CoinbaseSource{}), nil
}

func runBankDeposit(args []string) error {
	fs := flag.NewFlagSet("bank-deposit", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	asset := fs.String("asset", string(types.AssetSFG), "asset id")
	maxDisagreement := fs.String("oracle-max-disagreement", "0.02", "fractional bound beyond which the real CoinGecko/Coinbase quorum is treated as disagreeing")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	quorum, err := oracleQuorumFromFlags(*maxDisagreement)
	if err != nil {
		return err
	}
	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.BankDeposit(quorum, types.AssetID(*asset))
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}

func runBankWithdraw(args []string) error {
	fs := flag.NewFlagSet("bank-withdraw", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	asset := fs.String("asset", string(types.AssetSFG), "asset id")
	maxDisagreement := fs.String("oracle-max-disagreement", "0.02", "fractional bound beyond which the real CoinGecko/Coinbase quorum is treated as disagreeing")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	quorum, err := oracleQuorumFromFlags(*maxDisagreement)
	if err != nil {
		return err
	}
	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.BankWithdraw(quorum, types.AssetID(*asset))
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}
