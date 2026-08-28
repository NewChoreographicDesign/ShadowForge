package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

// parseNFTID decodes a hex string into an NFTID — the same 32-byte hex
// convention types.ParseHash already validates; NFTID and Hash share an
// identical underlying [32]byte array, so a validated Hash converts
// directly.
func parseNFTID(s string) (types.NFTID, error) {
	h, err := types.ParseHash(s)
	if err != nil {
		return types.NFTID{}, err
	}
	return types.NFTID(h), nil
}

func runIdentity(args []string) error {
	fs := flag.NewFlagSet("identity", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}
	fmt.Printf("identity: %s\n", ks.Identity())
	fmt.Printf("address: %s\n", types.AddressFromPubkey(ks.PublicKey()))
	fmt.Printf("public key: %s\n", ks.PublicKey())
	fmt.Printf("shielded public key: %s\n", hex.EncodeToString(ks.ShieldedPublicKey().Bytes()))
	return nil
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL, e.g. http://127.0.0.1:8081")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" {
		return fmt.Errorf("-query is required")
	}
	st, err := queryclient.New(*queryURL).Status(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("height: %d\n", st.Height)
	fmt.Printf("head hash: %s\n", st.HeadHash)
	fmt.Printf("genesis ms: %d\n", st.GenesisMs)
	return nil
}

func runTxStatus(args []string) error {
	fs := flag.NewFlagSet("tx-status", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	txidHex := fs.String("txid", "", "transaction id, hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *txidHex == "" {
		return fmt.Errorf("-query and -txid are required")
	}
	txid, err := types.ParseHash(*txidHex)
	if err != nil {
		return fmt.Errorf("-txid: %w", err)
	}
	st, err := queryclient.New(*queryURL).TxStatus(context.Background(), txid)
	if err != nil {
		return err
	}
	fmt.Printf("status: %s\n", st.Status)
	if st.Height != nil {
		fmt.Printf("height: %d\n", *st.Height)
	}
	return nil
}

func runNullifier(args []string) error {
	fs := flag.NewFlagSet("nullifier", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	hashHex := fs.String("hash", "", "nullifier, hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *hashHex == "" {
		return fmt.Errorf("-query and -hash are required")
	}
	h, err := types.ParseHash(*hashHex)
	if err != nil {
		return fmt.Errorf("-hash: %w", err)
	}
	spent, err := queryclient.New(*queryURL).NullifierSpent(context.Background(), h)
	if err != nil {
		return err
	}
	fmt.Printf("spent: %v\n", spent)
	return nil
}

func runNote(args []string) error {
	fs := flag.NewFlagSet("note", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	commitmentHex := fs.String("commitment", "", "note commitment, hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *commitmentHex == "" {
		return fmt.Errorf("-query and -commitment are required")
	}
	c, err := types.ParseHash(*commitmentHex)
	if err != nil {
		return fmt.Errorf("-commitment: %w", err)
	}
	exists, err := queryclient.New(*queryURL).NoteExists(context.Background(), c)
	if err != nil {
		return err
	}
	fmt.Printf("exists: %v\n", exists)
	return nil
}

func runNFT(args []string) error {
	fs := flag.NewFlagSet("nft", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	idHex := fs.String("id", "", "NFT id, hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *idHex == "" {
		return fmt.Errorf("-query and -id are required")
	}
	id, err := parseNFTID(*idHex)
	if err != nil {
		return fmt.Errorf("-id: %w", err)
	}
	nft, err := queryclient.New(*queryURL).NFT(context.Background(), id)
	if err != nil {
		if errors.Is(err, queryclient.ErrNotFound) {
			return fmt.Errorf("no NFT with id %s", id)
		}
		return err
	}
	fmt.Printf("id: %s\n", nft.ID)
	fmt.Printf("owner: %s\n", nft.Owner)
	fmt.Printf("minted at: %d\n", nft.MintedAt)
	fmt.Printf("tp: %d\n", nft.TP)
	fmt.Printf("slashed: %v\n", nft.Slashed)
	fmt.Printf("traits: %v\n", nft.Traits)
	return nil
}

func runHold(args []string) error {
	fs := flag.NewFlagSet("hold", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	idHex := fs.String("id", "", "hold id, hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *idHex == "" {
		return fmt.Errorf("-query and -id are required")
	}
	id, err := types.ParseHash(*idHex)
	if err != nil {
		return fmt.Errorf("-id: %w", err)
	}
	hold, err := queryclient.New(*queryURL).Hold(context.Background(), id)
	if err != nil {
		if errors.Is(err, queryclient.ErrNotFound) {
			return fmt.Errorf("no hold with id %s", id)
		}
		return err
	}
	fmt.Printf("hold id: %s\n", hold.HoldID)
	fmt.Printf("owner: %s\n", hold.Owner)
	fmt.Printf("asset: %s\n", hold.ExternalAsset)
	fmt.Printf("external amount: %s\n", hold.ExternalAmount)
	fmt.Printf("sfg issued: %d\n", hold.SFGIssued)
	fmt.Printf("status: %v\n", hold.Status)
	return nil
}

func runProposal(args []string) error {
	fs := flag.NewFlagSet("proposal", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	id := fs.String("id", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" || *id == "" {
		return fmt.Errorf("-query and -id are required")
	}
	p, err := queryclient.New(*queryURL).Proposal(context.Background(), *id)
	if err != nil {
		if errors.Is(err, queryclient.ErrNotFound) {
			return fmt.Errorf("no proposal with id %s", *id)
		}
		return err
	}
	printProposal(p)
	return nil
}

func runProposals(args []string) error {
	fs := flag.NewFlagSet("proposals", flag.ExitOnError)
	queryURL := fs.String("query", "", "pkg/query base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queryURL == "" {
		return fmt.Errorf("-query is required")
	}
	list, err := queryclient.New(*queryURL).Proposals(context.Background())
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("(no proposals)")
		return nil
	}
	for i, p := range list {
		if i > 0 {
			fmt.Println("---")
		}
		printProposal(p)
	}
	return nil
}

func printProposal(p queryclient.Proposal) {
	fmt.Printf("proposal id: %s\n", p.ProposalID)
	fmt.Printf("epoch: %d\n", p.Epoch)
	if p.ParamKey != "" {
		fmt.Printf("param key: %s\n", p.ParamKey)
		fmt.Printf("new value: %s\n", p.NewValue)
	}
	fmt.Printf("tallied: %v\n", p.Tallied)
	fmt.Printf("approve: %d\n", p.Approve)
	fmt.Printf("reject: %d\n", p.Reject)
	fmt.Printf("passed: %v\n", p.Passed)
	fmt.Printf("applied: %v\n", p.Applied)
	if p.MintAmount != 0 {
		fmt.Printf("mint amount: %d\n", p.MintAmount)
		fmt.Printf("mint staked: %v\n", p.MintStaked)
		if p.MintStaked {
			fmt.Printf("stake position commit: %s\n", p.StakePositionCommit)
		} else {
			fmt.Printf("mint out commit: %s\n", p.MintOutCommit)
		}
		fmt.Printf("mint applied: %v\n", p.MintApplied)
	}
	if p.SlashTargetNFT != "" {
		fmt.Printf("slash target: %s\n", p.SlashTargetNFT)
		fmt.Printf("slash burn: %v\n", p.SlashBurn)
		fmt.Printf("slash applied: %v\n", p.SlashApplied)
	}
}
