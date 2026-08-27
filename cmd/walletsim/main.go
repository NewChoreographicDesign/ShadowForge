// Command walletsim is the "client wallet simulator" spec 6 calls for in
// the Phase 2 four-node Docker network ("two civilian validators, one
// sentinel candidate, one client wallet simulator"). It connects to a
// bootstrap node and periodically submits lightweight Vote-kind
// transactions as TxOffer messages, simulating real wallet traffic without
// needing a full ZK proving setup (Kind Transfer requires the Groth16
// trusted setup; Vote does not).
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

func submitRandomVote(ctx context.Context, node *shadownet.Node) error {
	var proposalID [8]byte
	if _, err := rand.Read(proposalID[:]); err != nil {
		return err
	}
	t := types.ShieldedTx{
		TxID: types.SumHash(proposalID[:]),
		Kind: types.TxVote,
		Sig:  types.DilithiumSig("walletsim"),
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("sim-proposal"),
			Commitment: types.SumHash(proposalID[:], []byte("commit")),
		},
	}
	blob, err := json.Marshal(t)
	if err != nil {
		return err
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{TxBytes: blob})
	if err != nil {
		return err
	}
	errs := node.Broadcast(ctx, env)
	for peerID, sendErr := range errs {
		log.Printf("send to %s failed: %v", peerID, sendErr)
	}
	log.Printf("submitted simulated vote tx %s", t.TxID)
	return nil
}
