// Command node is the ShadowForge L1 validator node entrypoint (spec
// 18.1's cmd/node). It wires together the state, chain, transaction
// pipeline, and networking layers into a runnable process: a libp2p host
// exchanging heartbeats, transaction offers, and real BFT consensus
// messages (block proposals, stage votes, block announces) with its
// peers, gated through pkg/validator's propose/vote/commit state machine
// (spec section 5, including real Groth16 proof verification for Kind
// Transfer and real Dilithium-signed cross-node vote collection — spec
// 5.7's BFT quorum rule is enforced against genuine network peers, not
// just unit-tested in isolation).
//
// Outage/megabatch recovery (spec 5.6) and sentinel activation driving
// committee composition (spec 5.5) are real and wired into pkg/validator's
// round loop — see that package's doc for exactly what "wired" means here.
// -sentinel marks this process as a protocol-run sentinel: it only
// heartbeats (and therefore only participates in committee assignment)
// while pkg/validator's own SentinelManager has activated sentinels,
// determined from real observed online-civilian heartbeat counts, not a
// flag that merely changes a log line. This entrypoint still emits
// SilentPad padding traffic in sentinel mode (spec 15.4).
package main

import (
	"context"
	cryptorand "crypto/rand"
	"flag"
	"fmt"
	"log"
	mathrand "math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/validator"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// silentPadMeanInterval is the mean inter-arrival time for sentinel-emitted
// SilentPad padding traffic (spec 15.4).
const silentPadMeanInterval = 5 * time.Second

// defaultGenesisMs is this reference deployment's fixed genesis timestamp
// (2025-01-01T00:00:00Z, unix ms). Every node in a given network must
// agree on the exact same genesis time: it is hashed into the genesis
// block (pkg/chain.Open), and PrevHash-linked chain growth means nodes
// with different genesis blocks can never converge. -genesis-ms overrides
// this for a custom deployment or test network.
const defaultGenesisMs int64 = 1735689600000

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/4001", "libp2p listen multiaddr")
	bootstrap := flag.String("bootstrap", "", "comma-separated bootstrap peer multiaddrs")
	dataDir := flag.String("data", "", "Badger data directory (empty = in-memory)")
	sentinelFlag := flag.Bool("sentinel", false, "run as a protocol sentinel validator")
	skipZK := flag.Bool("skip-zk-setup", false, "skip the Groth16 trusted setup (Kind Transfer proofs will be rejected)")
	zkParamsPath := flag.String("zk-params", "", "path to a shared Groth16 parameters file (pkg/zk.System.WriteTo's format). If it exists, load it; if not, run a fresh trusted setup and write it there for other nodes/wallets to load. Empty (the default) runs an independent, unshared setup — fine for a single node, but a proof any wallet builds will never verify here unless every party loads the exact same params file (see 'wallet zk-setup' to generate one)")
	announceFile := flag.String("announce-file", "", "write this node's dialable multiaddr to this path once listening (for peer discovery over a shared volume, e.g. Docker Compose)")
	bootstrapFile := flag.String("bootstrap-file", "", "comma-separated paths to wait for and read bootstrap multiaddrs from (pairs with -announce-file on other nodes) — a full mesh among validators needs one entry per peer, since this reference build doesn't relay heartbeats or messages beyond directly connected peers")
	keyFile := flag.String("key-file", "", "path to this node's persisted Dilithium identity keypair (empty = generate a fresh, ephemeral identity every start)")
	genesisMs := flag.Int64("genesis-ms", defaultGenesisMs, "chain genesis time, unix milliseconds — every node in a network must use the same value")
	disableOracle := flag.Bool("disable-oracle", false, "disable real oracle price/ATR verification of BankDeposit/BankWithdraw claims (spec 11.3) — for isolated test networks without internet access")
	oracleMaxDisagreement := flag.String("oracle-max-disagreement", "0.02", "fractional bound (e.g. 0.02 = 2%) beyond which the oracle quorum's real sources are treated as disagreeing (spec 11.3)")
	queryListen := flag.String("query-listen", "127.0.0.1:8081", "address for the read-only HTTP query API (chain status, tx status, balances-equivalent lookups) — empty disables it. Binding a non-loopback address deliberately exposes it beyond this machine; see pkg/query's doc for exactly what it does and does not reveal")
	flag.Parse()

	role := "civilian"
	if *sentinelFlag {
		role = "sentinel"
	}
	log.Printf("ShadowForge node starting: listen=%s role=%s", *listen, role)

	encKey, err := loadOrCreateStateKey(*dataDir)
	if err != nil {
		log.Fatalf("state encryption key: %v", err)
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

	chn, err := chain.Open(store, *genesisMs)
	if err != nil {
		log.Fatalf("open chain: %v", err)
	}

	var zkSys *zk.System
	if !*skipZK {
		zkSys, err = loadOrCreateZKSystem(*zkParamsPath)
		if err != nil {
			log.Fatalf("zk setup: %v", err)
		}
	} else {
		log.Println("skipping ZK setup: Kind Transfer transactions will be rejected at Stage 1")
	}

	v := vault.New(vault.DefaultSplits())
	mempool := tx.NewMempool()

	// Real oracle quorum (spec 11.3): two independently-operated, real
	// public price feeds (CoinGecko and Coinbase, each with its own HTTP
	// client, response parsing, and real ATR computed from real historical
	// candles — see pkg/oracle). A BankDeposit/BankWithdraw whose claimed
	// price/ATR diverges from this quorum's agreed reading, or whose
	// sources genuinely disagree, is rejected/frozen by the pipeline
	// (pkg/tx's Stage 4) rather than trusted on the transaction's own say-so.
	var oracleQuorum *oracle.Quorum
	if *disableOracle {
		log.Println("oracle verification disabled (-disable-oracle): BankDeposit/BankWithdraw price/ATR claims will NOT be cross-checked against a real feed")
	} else {
		maxDisagreement, err := decimal.FromString(*oracleMaxDisagreement)
		if err != nil {
			log.Fatalf("parse -oracle-max-disagreement %q: %v", *oracleMaxDisagreement, err)
		}
		oracleQuorum = oracle.NewQuorum(maxDisagreement, oracle.CoinGeckoSource{}, oracle.CoinbaseSource{})
		log.Printf("real oracle quorum enabled: CoinGecko + Coinbase, max disagreement %s", *oracleMaxDisagreement)
	}

	pk, sk, err := loadOrCreateIdentity(*keyFile)
	if err != nil {
		log.Fatalf("load/create validator identity: %v", err)
	}

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

	for _, a := range shadownet.FullAddr(h) {
		log.Printf("listening: %s", a)
	}
	log.Printf("node ready: peer id=%s", h.ID())

	// Published before waiting on any -bootstrap-file: the listen address
	// is already known as soon as the host exists, independent of who
	// this node still needs to connect to. Two nodes each waiting on the
	// other's announce file (a full mesh's mutual bootstrapping) would
	// deadlock if this instead happened after the bootstrap wait below.
	if *announceFile != "" {
		if addrs := shadownet.FullAddr(h); len(addrs) > 0 {
			if err := writeAddrFile(*announceFile, addrs[0]); err != nil {
				log.Printf("write announce file %s: %v", *announceFile, err)
			}
		}
	}

	cfg := validator.DefaultConfig(consensus.GenesisTime(*genesisMs))
	vnode := validator.NewNode(cfg, h, nil, store, stateTree, chn, zkSys, v, oracleQuorum, mempool, pk, sk, *sentinelFlag, log.Printf)
	log.Printf("validator identity: %s", vnode.Identity())

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

	for _, path := range strings.Split(*bootstrapFile, ",") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		addr, err := waitForAddrFile(ctx, path)
		if err != nil {
			log.Printf("waiting for bootstrap file %s: %v", path, err)
			continue
		}
		if err := shadownet.Connect(ctx, h, addr); err != nil {
			log.Printf("bootstrap connect via file to %s failed: %v", addr, err)
			continue
		}
		log.Printf("connected to bootstrap peer %s (via %s)", addr, path)
	}

	vnode.Start(ctx)
	go epochLoop(ctx, consensus.GenesisTime(*genesisMs))
	go chainStatusLoop(ctx, vnode)

	if *queryListen != "" {
		qsrv := query.NewServer(store, chn, mempool, query.Config{
			ListenAddr: *queryListen,
			GenesisMs:  *genesisMs,
			Logf:       log.Printf,
		})
		if err := qsrv.Start(ctx); err != nil {
			log.Fatalf("start query API: %v", err)
		}
		if !isLoopback(*queryListen) {
			log.Printf("WARNING: query API bound to a non-loopback address (%s) — it is now reachable by anyone who can route to this machine. See pkg/query's doc comment for exactly what it does and does not expose.", *queryListen)
		}
	}
	if *sentinelFlag {
		go silent.RunPadGenerator(ctx, mathrand.New(mathrand.NewSource(time.Now().UnixNano())), silentPadMeanInterval, func() {
			emitSilentPad(ctx, vnode.Net())
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down")
}

// loadOrCreateIdentity loads a persisted Dilithium identity keypair from
// path, or generates a fresh one (persisting it to path, if path is
// non-empty) if none exists yet. A node's consensus identity
// (types.NFTID) is derived from its public key, so restarting with a
// different identity each time would make every one of its peers'
// committee assignments disagree about who it is — path lets a
// long-running node keep the same identity across restarts.
// loadOrCreateZKSystem loads a real, previously-generated Groth16 System
// from path (see zk.System.WriteTo's own doc on why sharing this matters:
// two independent zk.Setup() calls produce mutually incompatible keys, so
// a proof any wallet builds can only ever verify against a node that
// loaded the exact same params). An empty path always runs a fresh,
// unshared setup — the prior, still-supported single-node default. A
// non-empty path that doesn't exist yet runs a fresh setup and persists
// it there, so subsequent nodes (and any wallet) pointed at the same
// file share it; concurrent first-time startups against the same
// not-yet-existing path is a real, documented race this simple mechanism
// doesn't resolve — start the first node (or run `wallet zk-setup`)
// alone to generate it once, then point every other node/wallet at the
// resulting file.
func loadOrCreateZKSystem(path string) (*zk.System, error) {
	if path == "" {
		log.Println("running Groth16 trusted setup (development setup, unshared — see pkg/zk doc for the production-ceremony requirement)...")
		start := time.Now()
		sys, err := zk.Setup()
		if err != nil {
			return nil, err
		}
		log.Printf("zk setup complete in %s", time.Since(start))
		return sys, nil
	}

	if f, err := os.Open(path); err == nil {
		defer func() { _ = f.Close() }()
		sys, err := zk.ReadSystem(f)
		if err != nil {
			return nil, fmt.Errorf("load zk params from %s: %w", path, err)
		}
		log.Printf("loaded shared Groth16 params from %s", path)
		return sys, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read zk params file %s: %w", path, err)
	}

	log.Printf("running Groth16 trusted setup (development setup — see pkg/zk doc for the production-ceremony requirement) and saving it to %s for other nodes/wallets to share...", path)
	start := time.Now()
	sys, err := zk.Setup()
	if err != nil {
		return nil, err
	}
	log.Printf("zk setup complete in %s", time.Since(start))

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create zk params file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := sys.WriteTo(f); err != nil {
		return nil, fmt.Errorf("write zk params file %s: %w", path, err)
	}
	return sys, nil
}

// loadOrCreateStateKey returns the real symmetric key state.Open uses to
// encrypt note blobs and memos at rest. An empty dataDir means in-memory
// mode (tests, ephemeral nodes) — a fresh random key every start is
// correct there, since there is no on-disk data to ever reopen. A
// non-empty dataDir is a real persistence directory, so its key is
// persisted alongside it (dataDir/state.key, 0600): without this, a
// fresh random key generated on every process start could never decrypt
// data written under a previous run's key, silently making -data's
// entire persistence promise break on the very first restart — a real
// bug live auditing surfaced, not a documented boundary.
func loadOrCreateStateKey(dataDir string) (crypto.EncryptionKey, error) {
	if dataDir == "" {
		var key crypto.EncryptionKey
		if _, err := cryptorand.Read(key[:]); err != nil {
			return crypto.EncryptionKey{}, fmt.Errorf("generate ephemeral state encryption key: %w", err)
		}
		return key, nil
	}

	keyPath := filepath.Join(dataDir, "state.key")
	if b, err := os.ReadFile(keyPath); err == nil {
		if len(b) != len(crypto.EncryptionKey{}) {
			return crypto.EncryptionKey{}, fmt.Errorf("state key file %s is %d bytes, want %d", keyPath, len(b), len(crypto.EncryptionKey{}))
		}
		var key crypto.EncryptionKey
		copy(key[:], b)
		return key, nil
	} else if !os.IsNotExist(err) {
		return crypto.EncryptionKey{}, fmt.Errorf("read state key file %s: %w", keyPath, err)
	}

	var key crypto.EncryptionKey
	if _, err := cryptorand.Read(key[:]); err != nil {
		return crypto.EncryptionKey{}, fmt.Errorf("generate state encryption key: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return crypto.EncryptionKey{}, fmt.Errorf("create data dir %s: %w", dataDir, err)
	}
	if err := os.WriteFile(keyPath, key[:], 0o600); err != nil {
		return crypto.EncryptionKey{}, fmt.Errorf("write state key file %s: %w", keyPath, err)
	}
	return key, nil
}

func loadOrCreateIdentity(path string) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey, error) {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			pk, sk, err := decodeIdentity(b)
			if err != nil {
				return nil, nil, fmt.Errorf("decode identity file %s: %w", path, err)
			}
			return pk, sk, nil
		} else if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read identity file %s: %w", path, err)
		}
	}

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		return nil, nil, fmt.Errorf("generate identity: %w", err)
	}
	if path != "" {
		if err := writeIdentityFile(path, pk, sk); err != nil {
			return nil, nil, fmt.Errorf("persist identity file %s: %w", path, err)
		}
	}
	return pk, sk, nil
}

// identity files are [4-byte big-endian pubkey length][pubkey][privkey];
// the private key fills the remainder of the file, since Dilithium3's key
// sizes are fixed per mode but this avoids hard-coding them here.
func decodeIdentity(b []byte) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("identity file too short (%d bytes)", len(b))
	}
	pkLen := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if pkLen < 0 || 4+pkLen > len(b) {
		return nil, nil, fmt.Errorf("identity file has an invalid public key length %d", pkLen)
	}
	pk := crypto.DilithiumPublicKey(b[4 : 4+pkLen])
	sk := crypto.DilithiumPrivateKey(b[4+pkLen:])
	if len(sk) == 0 {
		return nil, nil, fmt.Errorf("identity file has no private key material")
	}
	return pk, sk, nil
}

func writeIdentityFile(path string, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey) error {
	buf := make([]byte, 4+len(pk)+len(sk))
	buf[0] = byte(len(pk) >> 24)
	buf[1] = byte(len(pk) >> 16)
	buf[2] = byte(len(pk) >> 8)
	buf[3] = byte(len(pk))
	copy(buf[4:], pk)
	copy(buf[4+len(pk):], sk)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

// isLoopback reports whether addr's host resolves to a loopback address
// (127.0.0.0/8 or ::1) — used only to decide whether to warn that the
// query API is now reachable from beyond this machine. An unresolvable
// or non-IP host (e.g. a bare hostname) is treated as non-loopback: warn
// rather than silently assume it's safe.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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

func epochLoop(ctx context.Context, genesis consensus.GenesisTime) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var lastEpoch = ^uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			epoch := consensus.CurrentEpoch(genesis, time.Now())
			if epoch != lastEpoch {
				log.Printf("epoch %d (duration %s)", epoch, consensus.EpochDuration(epoch))
				lastEpoch = epoch
			}
		}
	}
}

// chainStatusLoop periodically logs this node's real chain head, so an
// operator (or the Docker multi-node smoke test) can observe consensus
// genuinely advancing rather than trusting it silently.
func chainStatusLoop(ctx context.Context, vnode *validator.Node) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastHeight := ^uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h := vnode.Chain().HeadHeight()
			if h != lastHeight {
				log.Printf("chain height=%d hash=%s", h, vnode.Chain().HeadHash())
				lastHeight = h
			}
		}
	}
}
