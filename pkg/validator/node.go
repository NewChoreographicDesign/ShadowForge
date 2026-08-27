// Package validator implements the real cross-node consensus loop this
// build's architecture doc used to name as its one big remaining gap: a
// deterministically-assigned committee proposes and votes on one agreed
// batch per height, with genuine Dilithium signatures exchanged over the
// real libp2p network from pkg/net, gated through pkg/chain's
// independently-reverifying Append before any block becomes canonical.
//
// What is real here, concretely: the proposer is chosen the same way by
// every node (consensus.AssignCommittee, a pure function of the observed
// online set and height — see that package's doc for why). Every vote
// this package trusts has already been checked against a real Dilithium
// public key, not merely structurally present. A batch is applied
// tentatively (state.Txn, pkg/state.MerkleTree snapshot) and only ever
// committed once BFT quorum is independently reverified; anything that
// doesn't reach quorum is rolled back, not silently kept. A node that
// receives a BlockAnnounce for a batch it did not itself vote on replays
// the batch through the exact same pipeline and refuses to adopt the
// block if its own recomputed state root disagrees with what was
// announced.
//
// What is intentionally out of scope, documented rather than silently
// skipped: multi-block catch-up sync for a node that falls more than one
// block behind (see handleBlockAnnounce), and wiring pkg/consensus's
// already-implemented, already-tested outage/megabatch recovery into this
// loop (a separate, large piece of work — see docs/ARCHITECTURE.md).
package validator

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// Config tunes the round loop's timing.
type Config struct {
	// BatchInterval is how often the round loop checks whether this node
	// should propose the next block (spec 22's default BatchInterval).
	BatchInterval time.Duration
	// RoundTimeout is how long an in-flight round waits for quorum before
	// it is discarded (rolled back) and its transactions returned to the
	// mempool for retry — spec 22's default StageTimeout, scaled up
	// slightly since this covers a full propose-vote-commit round, not
	// one stage.
	RoundTimeout time.Duration
	// HeartbeatInterval is how often this node broadcasts its own
	// heartbeat and refreshes its self-entry in the online set.
	HeartbeatInterval time.Duration
	// OnlineTimeout is how long since a peer's last heartbeat before it
	// drops out of the online set used for committee assignment.
	OnlineTimeout time.Duration
	// Genesis is this chain's genesis time, for epoch computation
	// (consensus.CurrentEpoch).
	Genesis consensus.GenesisTime
	// MaxBatchSize caps how many mempool entries maybePropose will ever
	// consider in one block; <=0 means DefaultMaxBatchSize. This is a
	// secondary bound — MaxBatchBytes below is the real defense (see its
	// own doc for why a count alone can't safely bound serialized size).
	// A rejected batch's valid transactions go back into the mempool
	// (handleBlockProposal), so an unbounded drain lets one recurring bad
	// entry force an ever-larger batch to be reattempted every round.
	MaxBatchSize int
	// MaxBatchBytes caps the cumulative JSON-marshaled size of the
	// entries maybePropose drains into one block (Mempool.
	// DrainBatchBytes); <=0 means DefaultMaxBatchBytes. This is the real
	// defense: a real post-quantum Dilithium3 signature+pubkey alone is
	// several KB, so a few hundred otherwise-ordinary transactions can
	// exceed Badger's 1MB per-value limit well before MaxBatchSize's
	// count would ever trip — this build hit exactly that for real under
	// sustained traffic, and the resulting chain.Append failure
	// (reinserting the same oversized batch every round) formed a
	// livelock, not just a rejected round.
	MaxBatchBytes int
}

// DefaultMaxBatchSize is a conservative secondary cap on entry count.
const DefaultMaxBatchSize = 200

// DefaultMaxBatchBytes leaves comfortable margin under Badger's default
// 1MB (1048576 byte) per-value limit for the rest of the block's own
// fields (header, votes, JSON structure overhead) once the batch is
// embedded in it.
const DefaultMaxBatchBytes = 800 * 1024

func (c Config) maxBatchSize() int {
	if c.MaxBatchSize <= 0 {
		return DefaultMaxBatchSize
	}
	return c.MaxBatchSize
}

func (c Config) maxBatchBytes() int {
	if c.MaxBatchBytes <= 0 {
		return DefaultMaxBatchBytes
	}
	return c.MaxBatchBytes
}

// DefaultConfig returns the spec-22-aligned defaults.
func DefaultConfig(genesis consensus.GenesisTime) Config {
	return Config{
		BatchInterval:     time.Second,
		RoundTimeout:      8 * time.Second,
		HeartbeatInterval: consensus.HeartbeatInterval,
		OnlineTimeout:     consensus.OfflineWindow,
		Genesis:           genesis,
		MaxBatchSize:      DefaultMaxBatchSize,
		MaxBatchBytes:     DefaultMaxBatchBytes,
	}
}

// Logf is a pluggable logger; nil (the default from NewNode) uses log.Printf.
type Logf func(format string, args ...interface{})

type onlineInfo struct {
	lastBeat time.Time
	pubKey   crypto.DilithiumPublicKey
}

// Node runs the propose/vote/commit state machine for one validator
// process. It owns a libp2p-backed net.Node, the mempool, the pipeline's
// storage/state dependencies, and the chain it grows.
type Node struct {
	cfg     Config
	net     *shadownet.Node
	mempool *tx.Mempool
	store   *state.Store
	tree    *state.MerkleTree
	zkSys   *zk.System
	vlt     *vault.Vault
	chn     *chain.Chain
	// silentMon is spec 15.4's per-wallet rate monitor. Constructed
	// internally (not a NewNode parameter) since it's this node's own
	// runtime defense state, not an external dependency a caller owns —
	// same treatment as rounds/online below.
	silentMon *silent.RateMonitor

	identity types.NFTID
	pk       crypto.DilithiumPublicKey
	sk       crypto.DilithiumPrivateKey

	log Logf

	mu     sync.Mutex // guards online only
	online map[types.NFTID]onlineInfo

	// roundMu guards rounds and every mutation of tree/store/txn state
	// made while processing a round (handleBlockProposal, handleStageVote,
	// tryFinalizeLocked, sweepTimeouts, handleBlockAnnounce). Real
	// network delivery means these can fire concurrently from multiple
	// libp2p stream goroutines at once; without a single lock serializing
	// them, concurrent handlers race on the same *state.MerkleTree and
	// round bookkeeping. A separate mutex from mu (rather than reusing
	// it) avoids self-deadlock: this package's round-processing methods
	// call onlineSet/pubKeyLookup, which lock mu, while already holding
	// roundMu.
	roundMu sync.Mutex
	rounds  map[uint64]*round
}

// NewNode builds a validator.Node, constructing its own net.Node on h so
// the message handler (a method on the Node being built) can be wired at
// construction time. pk/sk are this node's real Dilithium identity
// keypair; the node's consensus identity (types.NFTID) is derived from the
// public key (types.NFTID(types.SumHash(pk))) — a genuine cryptographic
// binding, not an arbitrary label.
func NewNode(cfg Config, h host.Host, limiter *shadownet.RateLimiter, store *state.Store, tree *state.MerkleTree, chn *chain.Chain, zkSys *zk.System, vlt *vault.Vault, mempool *tx.Mempool, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, logf Logf) *Node {
	if logf == nil {
		logf = log.Printf
	}
	n := &Node{
		cfg:       cfg,
		mempool:   mempool,
		store:     store,
		tree:      tree,
		zkSys:     zkSys,
		vlt:       vlt,
		chn:       chn,
		silentMon: silent.NewRateMonitor(),
		identity:  types.NFTID(types.SumHash(pk)),
		pk:        pk,
		sk:        sk,
		log:       logf,
		online:    map[types.NFTID]onlineInfo{},
		rounds:    map[uint64]*round{},
	}
	n.net = shadownet.NewNode(h, limiter, n.handleMessage)
	n.recordOnline(n.identity, pk, time.Now())
	return n
}

// Identity is this node's consensus identity (derived from its Dilithium
// public key).
func (n *Node) Identity() types.NFTID { return n.identity }

// Chain exposes the underlying chain for read access (height, hash, head
// block) by callers such as cmd/node's logging.
func (n *Node) Chain() *chain.Chain { return n.chn }

// Mempool exposes the mempool so external submitters (a TxOffer handler
// outside this package, a local wallet-style API) can feed it — the round
// loop only ever drains it, never decides who may add to it.
func (n *Node) Mempool() *tx.Mempool { return n.mempool }

// Net exposes the underlying net.Node, e.g. for FullAddr/Host access by
// the caller's own logging.
func (n *Node) Net() *shadownet.Node { return n.net }

// Start launches the heartbeat and round loops; it returns immediately and
// runs until ctx is done.
func (n *Node) Start(ctx context.Context) {
	go n.heartbeatLoop(ctx)
	go n.roundLoop(ctx)
}

func (n *Node) recordOnline(id types.NFTID, pk crypto.DilithiumPublicKey, now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.online[id] = onlineInfo{lastBeat: now, pubKey: pk}
}

// onlineSet returns the sorted set of validator identities heartbeat-active
// within OnlineTimeout, for committee assignment.
func (n *Node) onlineSet(now time.Time) []types.NFTID {
	n.mu.Lock()
	ids := make([]types.NFTID, 0, len(n.online))
	for id, info := range n.online {
		if now.Sub(info.lastBeat) <= n.cfg.OnlineTimeout {
			ids = append(ids, id)
		}
	}
	n.mu.Unlock()
	return consensus.SortNFTIDs(ids)
}

// pubKeyLookup implements chain.PubKeyLookup against the online registry.
func (n *Node) pubKeyLookup(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	info, ok := n.online[id]
	if !ok {
		return nil, false
	}
	return info.pubKey, true
}

func (n *Node) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(n.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			n.recordOnline(n.identity, n.pk, now)
			env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
				NFT: n.identity, PubKey: []byte(n.pk), Timestamp: now.UnixMilli(),
			})
			if err != nil {
				n.log("validator: build heartbeat: %v", err)
				continue
			}
			n.net.Broadcast(ctx, env)
		}
	}
}
