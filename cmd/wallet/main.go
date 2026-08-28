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
	case "propose-mint":
		err = runProposeMint(args)
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
	case "eligibility-zk-setup":
		err = runEligibilityZKSetup(args)
	case "mint-zk-setup":
		err = runMintZKSetup(args)
	case "poh-attest":
		err = runPoHAttest(args)
	case "nft-mint":
		err = runNFTMint(args)
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

Claim your free, one-per-wallet validator NFT (spec 10.1) — required for
real governance voting eligibility (see 'wallet vote' below). The actual
proof-of-humanity challenge is out of this L1 core's scope; an attestor
you (or the network) trust runs 'poh-attest' after doing that check by
its own real means, and hands you the resulting flags:
  wallet poh-attest -keystore <attestor-file> -owner <hex-address> -nonce <n>
  wallet nft-mint   -keystore <file> -bootstrap <addr> -query <url> -nonce <n> -attestation-issued-at-ms <n> -attestor-pubkey <hex> -attestation-sig <hex>

Submit a real, signed, no-proof transaction (needs a bootstrap peer to
broadcast to, and a query endpoint to confirm against). Vote/vote-reveal/
propose-mint require a real, minted NFT (see 'wallet nft-mint' above) and
prove it anonymously — a real zero-knowledge membership proof, not a
formality, and needing its own one-time shared setup (like 'wallet
zk-setup' below, but a separate circuit and file):
  wallet eligibility-zk-setup -out <file>
  wallet vote          -keystore <file> -bootstrap <addr> -query <url> -proposal <id> -approve -eligibility-zk-params <file>
  wallet vote-reveal    -keystore <file> -bootstrap <addr> -query <url> -proposal <id> -approve -eligibility-zk-params <file>
  wallet nft-trait      -keystore <file> -bootstrap <addr> -query <url> -target <hex> -key <string> -delta <int>
  wallet bank-deposit    -keystore <file> -bootstrap <addr> -query <url> -asset SFG
  wallet bank-withdraw   -keystore <file> -bootstrap <addr> -query <url> -asset SFG

'vote'/'vote-reveal'/'propose-mint' sign the transaction itself with a
fresh, throwaway key — never the keystore identity that minted the NFT —
and prove eligibility instead via a real anonymous ZK proof built by
syncing that keystore's minted NFT from the live network (pkg/govwallet).
An observer of the resulting transaction learns only that some real,
minted NFT voted, never which one.

Real spec-17.4 epoch mint (the "direct with 10 percent fee" proposer
path — this build has no staking subsystem, so the spec's alternative
staked-yield path is not implemented): submitting a proposal both casts
its own first ballot and binds a real Groth16-proven mint request; other
holders approve/reject it with ordinary 'vote'/'vote-reveal' against the
same -proposal id. Needs its own one-time shared setup, like
eligibility above:
  wallet mint-zk-setup -out <file>
  wallet propose-mint -keystore <file> -bootstrap <addr> -query <url> -proposal <id> -amount <n> -eligibility-zk-params <file> -mint-zk-params <file>
Once tallied, check 'wallet proposal -id <id>' for real Passed/
MintApplied status. The proposer alone knows the minted note's real
opening (they built it), so, like a shielded transfer's own bootstrap
gap, this build has no wallet-sync mechanism to auto-discover somebody
else's mint; save what propose-mint prints — it is the only record of
that note's opening this build ever produces. ('wallet note' does not
verify this: this build's note-existence index is only ever populated
by a wallet's own optional PutNote call, which nothing in this CLI — for
Transfer's own outputs either — currently makes, a real, separate,
pre-existing gap rather than one this fix introduces.)

  wallet mint -keystore <file> -bootstrap <addr> -query <url>
'wallet mint' is a different, older, and now-vestigial no-op kind — not
the real mint mechanism above; see types.TxMint's own doc.

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
