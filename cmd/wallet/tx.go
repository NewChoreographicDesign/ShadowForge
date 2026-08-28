package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
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

func runVote(args []string) error {
	fs := flag.NewFlagSet("vote", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	proposal := fs.String("proposal", "", "proposal id (required)")
	approve := fs.Bool("approve", false, "cast an approve ballot (default: reject)")
	paramKey := fs.String("param-key", "", "optional governance.Params field this proposal changes (only matters on the first Vote for this proposal id)")
	newValue := fs.String("new-value", "", "new value for -param-key")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposal == "" {
		return fmt.Errorf("-proposal is required")
	}
	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.Vote(types.ID(*proposal), *approve, *paramKey, *newValue)
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
}

func runVoteReveal(args []string) error {
	fs := flag.NewFlagSet("vote-reveal", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	proposal := fs.String("proposal", "", "proposal id (required) — must match an earlier vote")
	approve := fs.Bool("approve", false, "must be the exact same value passed to the matching vote")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposal == "" {
		return fmt.Errorf("-proposal is required")
	}
	b, err := loadBuilder(*path, *fromStdin)
	if err != nil {
		return err
	}
	txn, err := b.VoteReveal(types.ID(*proposal), *approve)
	if err != nil {
		return err
	}
	return submitTx(context.Background(), &nf, txn)
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
