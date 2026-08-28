// Command wallet is Tier B priority #6: the real, end-user-facing CLI
// tying together every wallet-side package this session built — pkg/
// walletkey (real local identity), pkg/queryclient (real read access to
// a live node), pkg/txbuilder (the six no-proof ShieldedTx kinds), pkg/
// txclient (real submit-and-confirm over the actual libp2p wire
// protocol), and pkg/shieldedwallet (real Kind Transfer: proving,
// encryption, discovery). Nothing in this binary is a mock or a stub —
// every subcommand drives the same real code path a live validator node
// itself checks a submitted transaction against.
//
// It deliberately stays a thin composition layer: this file and its
// siblings contain flag parsing, prompting, and formatting only — every
// piece of real logic (signing, proving, encrypting, submitting,
// confirming) lives in the packages above, already tested independently
// of any CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "identity":
		err = runIdentity(args)
	case "status":
		err = runStatus(args)
	case "tx-status":
		err = runTxStatus(args)
	case "nullifier":
		err = runNullifier(args)
	case "note":
		err = runNote(args)
	case "nft":
		err = runNFT(args)
	case "hold":
		err = runHold(args)
	case "proposal":
		err = runProposal(args)
	case "proposals":
		err = runProposals(args)
	case "vote":
		err = runVote(args)
	case "vote-reveal":
		err = runVoteReveal(args)
	case "mint":
		err = runMint(args)
	case "nft-trait":
		err = runNFTTrait(args)
	case "bank-deposit":
		err = runBankDeposit(args)
	case "bank-withdraw":
		err = runBankWithdraw(args)
	case "balance":
		err = runBalance(args)
	case "transfer":
		err = runTransfer(args)
	case "zk-setup":
		err = runZKSetup(args)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "wallet: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `wallet — the real ShadowForge client: query a live node, submit real
transactions, and send/receive real shielded transfers.

Identity (offline, no network):
  wallet identity -keystore <file>

Read-only queries (talk to one live node's pkg/query API):
  wallet status      -query <url>
  wallet tx-status    -query <url> -txid <hex>
  wallet nullifier    -query <url> -hash <hex>
  wallet note         -query <url> -commitment <hex>
  wallet nft          -query <url> -id <hex>
  wallet hold         -query <url> -id <hex>
  wallet proposal     -query <url> -id <string>
  wallet proposals    -query <url>

Submit a real, signed, no-proof transaction (needs a bootstrap peer to
broadcast to, and a query endpoint to confirm against):
  wallet vote          -keystore <file> -bootstrap <addr> -query <url> -proposal <id> -approve
  wallet vote-reveal    -keystore <file> -bootstrap <addr> -query <url> -proposal <id> -approve
  wallet mint           -keystore <file> -bootstrap <addr> -query <url>
  wallet nft-trait      -keystore <file> -bootstrap <addr> -query <url> -target <hex> -key <string> -delta <int>
  wallet bank-deposit    -keystore <file> -bootstrap <addr> -query <url> -asset SFG
  wallet bank-withdraw   -keystore <file> -bootstrap <addr> -query <url> -asset SFG

Real shielded transfers (Kind Transfer — a genuine Groth16 proof; needs
a shared params file every validator and wallet load identically — run
'wallet zk-setup' once to generate one if the network doesn't have one
yet, matching cmd/node's own -zk-params flag):
  wallet zk-setup   -out <file>
  wallet balance    -keystore <file> -query <url>
  wallet transfer   -keystore <file> -bootstrap <addr> -query <url> -zk-params <file> -to <hex-x25519-pubkey> -amount <n> -fee <n>

All commands prompt for a keystore passphrase on the real terminal
(input hidden) where one is needed. Pass -passphrase-stdin to read it
from stdin instead, one line, for scripting.

A real, disclosed limitation shared by "balance" and "transfer": this
build has no on-chain mechanism that originates a wallet's very first
shielded note (see pkg/shieldedwallet's own doc) — a brand-new wallet
genuinely has a balance of 0 until it receives a real transfer from one
that already holds spendable notes.`)
}
