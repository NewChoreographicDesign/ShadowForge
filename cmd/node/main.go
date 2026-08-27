// Command node is the ShadowForge L1 validator node entrypoint (spec
// 18.1's cmd/node). It wires together the state, consensus, transaction
// pipeline, and networking layers into a runnable process: a libp2p host
// gossiping heartbeats and transaction offers, a 1-second batcher running
// every admitted transaction through the five-stage pipeline, and the
// epoch/revolver/sentinel bookkeeping described in spec section 5.
//
// Scope note: this wires up everything through Stage 5 commit for
// transactions this node itself received and validated (spec 5.3's five
// stages, including real Groth16 proof verification for Kind Transfer).
// Cross-process BFT vote collection — gathering StageVote signatures from
// other physical validators over the wire and only committing once
// consensus.BFTQuorumMet holds across them (spec 5.7) — is implemented and
// unit-tested in pkg/consensus, but this entrypoint does not yet wire that
// exchange over the network; each node currently finalizes its own
// admitted batch locally. That network-vote-collection wiring is the next
// integration step beyond this reference build, not a gap in the
// consensus logic itself.
package main

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// silentPadMeanInterval is the mean inter-arrival time for sentinel-emitted
// SilentPad padding traffic (spec 15.4).
const silentPadMeanInterval = 5 * time.Second

const batchInterval = time.Second // spec 22 governance default

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/4001", "libp2p listen multiaddr")
	bootstrap := flag.String("bootstrap", "", "comma-separated bootstrap peer multiaddrs")
	dataDir := flag.String("data", "", "Badger data directory (empty = in-memory)")
	sentinelFlag := flag.Bool("sentinel", false, "run as a protocol sentinel validator")
	skipZK := flag.Bool("skip-zk-setup", false, "skip the Groth16 trusted setup (Kind Transfer proofs will be rejected)")
	announceFile := flag.String("announce-file", "", "write this node's dialable multiaddr to this path once listening (for peer discovery over a shared volume, e.g. Docker Compose)")
	bootstrapFile := flag.String("bootstrap-file", "", "read a bootstrap multiaddr from this path, waiting for it to appear (pairs with -announce-file on another node)")
	flag.Parse()

	role := "civilian"
	if *sentinelFlag {
		role = "sentinel"
	}
	log.Printf("ShadowForge node starting: listen=%s role=%s", *listen, role)

	var encKey [32]byte
	if _, err := cryptorand.Read(encKey[:]); err != nil {
		log.Fatalf("generate state encryption key: %v", err)
	}
	store, err := state.Open(*dataDir, *dataDir == "", encKey)
	if err != nil {
		log.Fatalf("open state store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close state store: %v", err)
		}
	}()
	stateTree := state.NewMerkleTree()

	var zkSys *zk.System
	if !*skipZK {
		log.Println("running Groth16 trusted setup (development setup — see pkg/zk doc for the production-ceremony requirement)...")
		start := time.Now()
		zkSys, err = zk.Setup()
		if err != nil {
			log.Fatalf("zk setup: %v", err)
		}
		log.Printf("zk setup complete in %s", time.Since(start))
	} else {
		log.Println("skipping ZK setup: Kind Transfer transactions will be rejected at Stage 1")
	}

	v := vault.New(vault.DefaultSplits())
	pipeline := tx.NewPipeline(tx.Deps{Store: store, StateTree: stateTree, ZK: zkSys, Vault: v, Now: time.Now})
	mempool := tx.NewMempool()
	revolver := consensus.NewRevolver()
	sentinels := consensus.NewSentinelManager()

	h, err := shadownet.NewHost(*listen)
	if err != nil {
		log.Fatalf("create libp2p host: %v", err)
	}
	defer func() {
		if err := h.Close(); err != nil {
			log.Printf("close libp2p host: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node := shadownet.NewNode(h, nil, func(p peer.ID, env shadownet.Envelope) {
		switch env.Type {
		case shadownet.MsgHeartbeat:
			var hb shadownet.HeartbeatPayload
			if err := json.Unmarshal(env.Payload, &hb); err != nil {
				log.Printf("bad heartbeat from %s: %v", p, err)
				return
			}
			handleHeartbeat(revolver, hb)

		case shadownet.MsgTxOffer:
			var offer shadownet.TxOfferPayload
			if err := json.Unmarshal(env.Payload, &offer); err != nil {
				log.Printf("bad tx offer from %s: %v", p, err)
				return
			}
			var t types.ShieldedTx
			if err := json.Unmarshal(offer.TxBytes, &t); err != nil {
				log.Printf("bad tx blob from %s: %v", p, err)
				return
			}
			if err := mempool.Submit(t, time.Now()); err != nil {
				log.Printf("tx offer from %s not admitted: %v", p, err)
			}

		default:
			// BlockAnnounce, StageVote, MegabatchPart, ContainerSync,
			// SilentPad: accepted on the wire (rate-limited like every
			// other message type) but not yet acted on by this
			// entrypoint — see the package doc's cross-process BFT note.
		}
	})

	for _, addr := range strings.Split(*bootstrap, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if err := shadownet.Connect(ctx, h, addr); err != nil {
			log.Printf("bootstrap connect to %s failed: %v", addr, err)
			continue
		}
		log.Printf("connected to bootstrap peer %s", addr)
	}

	if *bootstrapFile != "" {
		addr, err := waitForAddrFile(ctx, *bootstrapFile)
		if err != nil {
			log.Printf("waiting for bootstrap file %s: %v", *bootstrapFile, err)
		} else if err := shadownet.Connect(ctx, h, addr); err != nil {
			log.Printf("bootstrap connect via file to %s failed: %v", addr, err)
		} else {
			log.Printf("connected to bootstrap peer %s (via %s)", addr, *bootstrapFile)
		}
	}

	genesis := consensus.GenesisTime(time.Now().UnixMilli())

	go heartbeatLoop(ctx, node)
	go batchLoop(ctx, mempool, pipeline)
	go epochLoop(ctx, genesis, revolver, sentinels)
	if *sentinelFlag {
		go silent.RunPadGenerator(ctx, mathrand.New(mathrand.NewSource(time.Now().UnixNano())), silentPadMeanInterval, func() {
			emitSilentPad(ctx, node)
		})
	}

	for _, a := range shadownet.FullAddr(h) {
		log.Printf("listening: %s", a)
	}
	log.Printf("node ready: peer id=%s", h.ID())

	if *announceFile != "" {
		if addrs := shadownet.FullAddr(h); len(addrs) > 0 {
			if err := writeAddrFile(*announceFile, addrs[0]); err != nil {
				log.Printf("write announce file %s: %v", *announceFile, err)
			}
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down")
}

// writeAddrFile atomically publishes this node's dialable multiaddr so a
// sibling container on a shared volume can discover it without a
// preconfigured peer ID (Docker Compose has no built-in libp2p discovery).
func writeAddrFile(path, addr string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(addr), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// waitForAddrFile polls for path to appear (another node's writeAddrFile
// call racing this one at container start-up) and returns its contents.
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

func handleHeartbeat(revolver *consensus.Revolver, hb shadownet.HeartbeatPayload) {
	var nftID types.NFTID
	copy(nftID[:], []byte(hb.NFT))
	revolver.RequestJoin(types.QueueItem{
		NFT:      nftID,
		JoinedAt: hb.Timestamp,
		LastBeat: hb.Timestamp,
	}, time.Now())
}

func heartbeatLoop(ctx context.Context, node *shadownet.Node) {
	ticker := time.NewTicker(consensus.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
				NFT:       node.Host.ID().String(),
				Timestamp: time.Now().UnixMilli(),
			})
			if err != nil {
				log.Printf("build heartbeat: %v", err)
				continue
			}
			node.Broadcast(ctx, env)
		}
	}
}

// emitSilentPad broadcasts one null ZK padding message (spec 15.4:
// sentinels/Vault keep circuits warm and absorb burst load). The nonce is
// cosmetic — SilentPad carries no proof or value, so it never touches the
// pipeline — but it is still drawn from crypto/rand rather than the
// generator's math/rand source, since it becomes wire content other nodes
// observe.
func emitSilentPad(ctx context.Context, node *shadownet.Node) {
	var nonce [16]byte
	if _, err := cryptorand.Read(nonce[:]); err != nil {
		log.Printf("silent pad nonce: %v", err)
		return
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgSilentPad, shadownet.SilentPadPayload{Nonce: nonce[:]})
	if err != nil {
		log.Printf("build silent pad: %v", err)
		return
	}
	node.Broadcast(ctx, env)
}

func batchLoop(ctx context.Context, mempool *tx.Mempool, pipeline *tx.Pipeline) {
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries := mempool.DrainBatch(0)
			if len(entries) == 0 {
				continue
			}
			for _, r := range pipeline.ProcessBatch(entries) {
				if r.Error != nil {
					log.Printf("tx %s rejected: %v", r.Tx.TxID, r.Error)
				} else {
					log.Printf("tx %s committed (kind=%s)", r.Tx.TxID, r.Tx.Kind)
				}
			}
		}
	}
}

func epochLoop(ctx context.Context, genesis consensus.GenesisTime, revolver *consensus.Revolver, sentinels *consensus.SentinelManager) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var lastEpoch = ^uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			epoch := consensus.CurrentEpoch(genesis, now)
			if epoch != lastEpoch {
				log.Printf("epoch %d (duration %s)", epoch, consensus.EpochDuration(epoch))
				lastEpoch = epoch
			}
			online := revolver.Online(now)
			switch sentinels.Evaluate(online, now.UnixMilli()) {
			case consensus.ActionActivate:
				log.Printf("SENTINELS ACTIVATED: only %d online civilian validators (threshold %d)", online, consensus.SentinelThreshold)
			case consensus.ActionWithdraw:
				log.Printf("sentinels withdrawing: %d online civilian validators recovered", online)
			}
		}
	}
}
