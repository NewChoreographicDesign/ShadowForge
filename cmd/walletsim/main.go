// Command walletsim is the "client wallet simulator" spec 6 calls for in
// the Phase 2 four-node Docker network ("two civilian validators, one
// sentinel candidate, one client wallet simulator"). It connects to a
// bootstrap node and periodically submits lightweight Vote-kind
// transactions as TxOffer messages, simulating real wallet traffic without
// needing a full ZK proving setup (Kind Transfer requires the Groth16
// trusted setup; Vote does not).
//
// A real, disclosed limitation as of this build's real voter-eligibility
// check (pkg/tx's requireEligibleVoter, closing a real Sybil-voting gap):
// every ballot this tool casts will now be rejected by any node that
// enforces it, since submitRandomVote's fresh, throwaway-per-session
// identity (see that function's own doc on why that's deliberate) never
// holds a real, PoH-verified ValidatorNFT. This tool still exercises the
// real commit-reveal wire mechanics and signature checks end to end —
// useful for load/liveness testing the pipeline's earlier stages — but
// no longer demonstrates a ballot actually being tallied. Giving it a
// persistent, pre-minted identity would fix that at the cost of the
// very throwaway-identity behavior it exists to model; this reference
// build leaves that tension for whoever wires a real anonymous
// eligibility proof (see pkg/tx's own doc on TxVote) rather than
// resolving it here.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/4010", "libp2p listen multiaddr")
	bootstrap := flag.String("bootstrap", "", "bootstrap peer multiaddr")
	bootstrapFile := flag.String("bootstrap-file", "", "read a bootstrap multiaddr from this path, waiting for it to appear")
	interval := flag.Duration("interval", 3*time.Second, "how often to submit a simulated transaction")
	flag.Parse()

	h, err := shadownet.NewHost(*listen)
	if err != nil {
		log.Fatalf("create libp2p host: %v", err)
	}
	defer func() {
		if err := h.Close(); err != nil {
			log.Printf("close libp2p host: %v", err)
		}
	}()

	node := shadownet.NewNode(h, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrapAddr := *bootstrap
	if bootstrapAddr == "" && *bootstrapFile != "" {
		addr, err := waitForAddrFile(ctx, *bootstrapFile)
		if err != nil {
			log.Fatalf("waiting for bootstrap file %s: %v", *bootstrapFile, err)
		}
		bootstrapAddr = addr
	}
	if bootstrapAddr != "" {
		if err := shadownet.Connect(ctx, h, bootstrapAddr); err != nil {
			log.Fatalf("connect to bootstrap %s: %v", bootstrapAddr, err)
		}
		log.Printf("wallet simulator connected to %s", bootstrapAddr)
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := submitRandomVote(ctx, node); err != nil {
					log.Printf("submit failed: %v", err)
				}
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("wallet simulator shutting down")
}

// waitForAddrFile polls for path to appear, mirroring cmd/node's helper of
// the same purpose (kept duplicated rather than shared to keep each binary
// a single, independent file for this reference build).
func waitForAddrFile(ctx context.Context, path string) (string, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b)), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for %s", path)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// submitRandomVote simulates one real voter session: a fresh, throwaway
// Dilithium identity (spec's own privacy design — "Wallets create
// throw-away 'mirror' addresses for each session and burn them afterward",
// docs/SPEC_SOURCE.md — not just a workaround for "one NFT, one vote"
// rejecting a reused identity's second ballot) casts a real sealed
// TxVote ballot via types.ComputeVoteCommitment, then reveals it with a
// real TxVoteReveal — the same commit-reveal cycle pkg/tx's pipeline
// actually enforces, exercised here as one full, real ballot instead of
// an ever-growing pile of instantly-rejected duplicates from one reused
// identity.
func submitRandomVote(ctx context.Context, node *shadownet.Node) error {
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		return fmt.Errorf("generate throwaway voter key: %w", err)
	}
	voter := types.NFTID(types.SumHash([]byte(pk)))

	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		return err
	}
	nonce := types.Hash(nonceBytes)
	var approveByte [1]byte
	if _, err := rand.Read(approveByte[:]); err != nil {
		return err
	}
	approve := approveByte[0]%2 == 0
	commitment := types.ComputeVoteCommitment(voter, approve, nonce)

	commitTx := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("sim-proposal"),
			Commitment: commitment,
		},
		// TxID = Hash(proof || commitments || nullifier) per spec 4.1;
		// VotePublicInputs isn't part of that hash, so without a
		// per-submission Nullifier here every commit tx would collide on
		// the same TxID (fields Proof/Commitments are otherwise unset for
		// Vote kind). commit and reveal need *distinct* nullifiers too —
		// reusing one would collide the pair's own TxIDs with each other
		// and have the mempool's TxID-based dedup silently drop the
		// second one as a duplicate of the first.
		Nullifier: types.SumHash(nonce[:], []byte("commit")),
	}
	if err := signAndSend(ctx, node, pk, sk, commitTx); err != nil {
		return fmt.Errorf("submit ballot commitment: %w", err)
	}
	log.Printf("submitted simulated vote commitment for voter %s (approve=%v)", voter, approve)

	revealTx := types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("sim-proposal"),
			Approve:    approve,
			Nonce:      nonce,
		},
		Nullifier: types.SumHash(nonce[:], []byte("reveal")), // distinct from commitTx's — see its comment
	}
	if err := signAndSend(ctx, node, pk, sk, revealTx); err != nil {
		return fmt.Errorf("submit ballot reveal: %w", err)
	}
	log.Printf("submitted simulated vote reveal for voter %s", voter)
	return nil
}

// signAndSend computes t's TxID (spec 4.1), signs it with sk, and
// broadcasts it as a TxOffer.
func signAndSend(ctx context.Context, node *shadownet.Node, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, t types.ShieldedTx) error {
	t.TxID = types.ComputeTxID(t.Proof, t.Commitments, t.Nullifier)
	sig, err := crypto.DilithiumSign(sk, t.TxID[:])
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	t.Sig = types.DilithiumSig(sig)
	t.SignerPubKey = []byte(pk)

	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: t})
	if err != nil {
		return err
	}
	errs := node.Broadcast(ctx, env)
	for peerID, sendErr := range errs {
		log.Printf("send to %s failed: %v", peerID, sendErr)
	}
	return nil
}
