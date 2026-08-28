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
// Sentinel activation (spec 5.5) and outage/megabatch recovery (spec 5.6)
// are wired in for real too, not just unit-tested in pkg/consensus in
// isolation: every node evaluates consensus.SentinelManager off its own
// real online-civilian heartbeat count each heartbeat tick (a
// sentinel-flagged node genuinely stands down — stops heartbeating and
// therefore drops out of committee assignment — while not needed, rather
// than "sentinel" being a label that only affects logging), and
// consensus.OutageController off real missing-heartbeat history each
// round tick. While an outage is declared, incoming transactions are
// diverted to the backlog instead of the live mempool (handleMessage's
// MsgTxOffer case), and maybePropose builds dual-track batches (Track A
// live + Track B drained backlog, still bounded by the same MaxBatchBytes
// budget that protects every ordinary batch) until a clean dual-track
// cycle reaches real BFT quorum and OutageFlag clears.
//
// What is intentionally out of scope, documented rather than silently
// skipped: multi-block catch-up sync for a node that falls more than one
// block behind (see handleBlockAnnounce), and MegabatchPart's chunked
// wire-format reassembly for a recovery batch too large to fit even a
// single MaxBatchBytes-bounded proposal (see pkg/net/message.go's doc) —
// a real megabatch fits the existing single-proposal pipeline for any
// backlog depth this build's own MaxBatchBytes budget can absorb per
// round; splitting one across multiple wire messages is a separate,
// larger undertaking.
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
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
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
	lastBeat   time.Time
	pubKey     crypto.DilithiumPublicKey
	isSentinel bool
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
	// zkTree/zkRoots are the real, canonical BN254/MiMC commitment tree
	// and its historical root set (zk.RootHistory) — the closed gap that
	// used to let a Transfer proof anchor to any root the prover claimed,
	// verified for internal consistency but never checked against what
	// the network actually committed. Constructed internally, not a
	// NewNode parameter: like governanceParams below, this is the node's
	// own consensus-derived runtime state, built fresh (matching tree's
	// own fresh-per-process construction above) rather than an external
	// dependency a caller configures.
	zkTree  *zk.Tree
	zkRoots *zk.RootHistory
	// oracleQuorum is the real quorum-verified price/ATR feed the pipeline
	// cross-checks BankDeposit/BankWithdraw claims against (spec 11.3). Nil
	// disables the cross-check entirely (e.g. a local test network with no
	// real oracle configured) — see tx.Deps.Oracle's own doc for what that
	// means concretely.
	oracleQuorum *oracle.Quorum
	// trustedPoHAttestors is the real proof-of-humanity attestor public
	// key set (spec 10.1) Kind NFTMint's signed attestation must be
	// signed by one of — see tx.Deps.TrustedPoHAttestors' own doc on why
	// empty means every mint attempt is rejected (fail closed), not
	// "check disabled".
	trustedPoHAttestors []crypto.DilithiumPublicKey
	// governanceParams is this node's live governance parameter set (spec
	// 9.1/17.4), starting at governance.Default() and mutated in place by
	// the pipeline (tx.Deps.Governance) the moment a ProposalParamChange
	// proposal passes tally — see tx.Deps.Governance's own doc. Constructed
	// internally, not a NewNode parameter: like silentMon below, it's this
	// node's own consensus-derived runtime state, not an external
	// dependency a caller configures.
	governanceParams *governance.Params
	// silentMon is spec 15.4's per-wallet rate monitor. Constructed
	// internally (not a NewNode parameter) since it's this node's own
	// runtime defense state, not an external dependency a caller owns —
	// same treatment as rounds/online below.
	silentMon *silent.RateMonitor
	// sentinels tracks spec 5.5's sentinel activation state (10 protocol
	// -run validators activate when online civilians drop below 10, and
	// withdraw once the civilian queue recovers) — real evaluation driven
	// off real heartbeat data in heartbeatLoop, not a static role label.
	// Constructed internally: this node's own runtime state, evaluated
	// identically by every node from its own local view, same as online.
	sentinels *consensus.SentinelManager
	// outage tracks spec 5.6's outage/megabatch recovery pipeline: detect
	// outage from real missing-heartbeat data, backlog incoming
	// transactions instead of admitting them live, and build dual-track
	// recovery batches (Track A live + Track B drained backlog) once
	// enough of the network is heartbeating again. Constructed internally
	// for the same reason as sentinels above.
	outage *consensus.OutageController
	// isSentinel is this node's own configured role (cmd/node's -sentinel
	// flag). A sentinel-flagged node only heartbeats/participates in
	// committee assignment while n.sentinels.Active() — see
	// heartbeatLoop — so it genuinely stands down when not needed rather
	// than always running with role only affecting logging.
	isSentinel bool

	identity types.NFTID
	pk       crypto.DilithiumPublicKey
	sk       crypto.DilithiumPrivateKey

	log Logf

	mu     sync.Mutex // guards online and everSeen
	online map[types.NFTID]onlineInfo
	// everSeen records the last time each identity was ever heartbeat-seen,
	// independent of online's OnlineTimeout pruning — outageBaseline uses
	// it to compute "more than 50% of the last-known online set is missing
	// heartbeats" (spec 5.6), which needs a longer-lived baseline than
	// online's own live-committee-assignment window.
	everSeen map[types.NFTID]time.Time

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
func NewNode(cfg Config, h host.Host, limiter *shadownet.RateLimiter, store *state.Store, tree *state.MerkleTree, chn *chain.Chain, zkSys *zk.System, vlt *vault.Vault, oracleQuorum *oracle.Quorum, trustedPoHAttestors []crypto.DilithiumPublicKey, mempool *tx.Mempool, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, isSentinel bool, logf Logf) *Node {
	if logf == nil {
		logf = log.Printf
	}
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		// A fresh, fully-zero-padded tree computing its root is a pure,
		// deterministic operation with no I/O or randomness — this
		// should be unreachable. If it somehow isn't, every node still
		// needs to agree, so fall back to the deterministic zero value
		// rather than letting divergent nodes seed different histories;
		// NewNode's signature has no error return to surface this
		// through instead.
		logf("validator: BUG: fresh zk.Tree root computation failed (%v); seeding RootHistory with the zero element", err)
	}
	n := &Node{
		cfg:                 cfg,
		mempool:             mempool,
		store:               store,
		tree:                tree,
		zkSys:               zkSys,
		vlt:                 vlt,
		chn:                 chn,
		zkTree:              zkTree,
		zkRoots:             zk.NewRootHistory(initialRoot),
		oracleQuorum:        oracleQuorum,
		trustedPoHAttestors: trustedPoHAttestors,
		governanceParams: func() *governance.Params {
			p := governance.Default()
			return &p
		}(),
		silentMon:  silent.NewRateMonitor(),
		sentinels:  consensus.NewSentinelManager(),
		outage:     consensus.NewOutageController(consensus.DefaultOutageThresholds()),
		isSentinel: isSentinel,
		identity:   types.NFTID(types.SumHash(pk)),
		pk:         pk,
		sk:         sk,
		log:        logf,
		online:     map[types.NFTID]onlineInfo{},
		everSeen:   map[types.NFTID]time.Time{},
		rounds:     map[uint64]*round{},
	}
	n.net = shadownet.NewNode(h, limiter, n.handleMessage)
	if !isSentinel {
		// A sentinel-flagged node only joins the online set once sentinels
		// are actually activated (see heartbeatLoop) — it must not record
		// itself here at construction time the way a civilian does.
		n.recordOnline(n.identity, pk, isSentinel, time.Now())
	}
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

func (n *Node) recordOnline(id types.NFTID, pk crypto.DilithiumPublicKey, isSentinel bool, now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.online[id] = onlineInfo{lastBeat: now, pubKey: pk, isSentinel: isSentinel}
	n.everSeen[id] = now
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

// onlineCivilianCount counts online (within OnlineTimeout), non-sentinel
// identities — the input consensus.SentinelManager.Evaluate needs (spec
// 5.5's "if revolver.Online() < 10" is meant as the civilian queue, not
// sentinels themselves, since sentinels activating because sentinels are
// scarce would be circular).
func (n *Node) onlineCivilianCount(now time.Time) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	count := 0
	for _, info := range n.online {
		if !info.isSentinel && now.Sub(info.lastBeat) <= n.cfg.OnlineTimeout {
			count++
		}
	}
	return count
}

// outageBaselineWindow bounds how long an identity's last heartbeat is
// still counted toward outageBaseline's "last-known online set" — long
// enough to span a real outage's detection-to-recovery cycle, short enough
// that a validator gone for good eventually stops being counted as
// "missing" forever.
const outageBaselineWindow = 10 * time.Minute

// outageBaseline computes spec 5.6's detection inputs from real heartbeat
// history: lastKnownOnline is how many distinct identities have
// heartbeated at all within outageBaselineWindow, and missing is how many
// of those are not currently within OnlineTimeout (i.e. have gone quiet
// more recently than that, but not so long ago they've aged out of the
// baseline entirely).
func (n *Node) outageBaseline(now time.Time) (lastKnownOnline, missing int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for id, seenAt := range n.everSeen {
		if now.Sub(seenAt) > outageBaselineWindow {
			continue
		}
		lastKnownOnline++
		if info, ok := n.online[id]; !ok || now.Sub(info.lastBeat) > n.cfg.OnlineTimeout {
			missing++
		}
	}
	return lastKnownOnline, missing
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

// heartbeatLoop broadcasts this node's own heartbeat immediately on
// entry, then every HeartbeatInterval thereafter. The immediate first
// broadcast matters for real safety, not just responsiveness: a
// time.Ticker's first tick only fires *after* one full interval, so
// without it every node would spend up to a whole HeartbeatInterval
// (10s by default) believing it is the only validator online. A real
// live-multi-process test hit exactly that cold-start blind window: two
// nodes, each still seeing only itself, independently proposed and
// self-committed *different* blocks for the same height — a real fork.
// consensus.MinCommitteeSize now refuses a committee below 2 regardless
// (the actual safety fix, since nothing can otherwise guarantee every
// node's heartbeat has round-tripped by the time it first proposes), but
// shrinking the blind window this way is still a real, direct
// improvement to how long a healthy network takes to safely converge.
func (n *Node) heartbeatLoop(ctx context.Context) {
	n.sendHeartbeat(ctx)

	ticker := time.NewTicker(n.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat records this node online (unless it's a standing-down
// sentinel — spec 5.5) and broadcasts a real heartbeat to its peers.
func (n *Node) sendHeartbeat(ctx context.Context) {
	now := time.Now()
	n.evaluateSentinels(now)
	if n.isSentinel && !n.sentinels.Active() {
		// Sentinels stand down while not needed (spec 5.5): skip
		// recording ourselves online and broadcasting a heartbeat
		// entirely, so this node genuinely drops out of every peer's
		// online set — and therefore out of committee assignment —
		// until sentinels are next activated, rather than always
		// participating with "sentinel" only ever affecting a log line.
		return
	}
	n.recordOnline(n.identity, n.pk, n.isSentinel, now)
	env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
		NFT: n.identity, PubKey: []byte(n.pk), Timestamp: now.UnixMilli(), IsSentinel: n.isSentinel,
	})
	if err != nil {
		n.log("validator: build heartbeat: %v", err)
		return
	}
	n.net.Broadcast(ctx, env)
}
