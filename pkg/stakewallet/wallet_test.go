package stakewallet_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/govwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/stakewallet"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	eligOnce sync.Once
	eligSys  *zk.EligibilitySystem
	eligErr  error

	stakeOnce sync.Once
	stakeSys  *zk.StakeSystem
	stakeErr  error

	unstakeOnce sync.Once
	unstakeSys  *zk.UnstakeSystem
	unstakeErr  error
)

func getEligibilitySystem(t *testing.T) *zk.EligibilitySystem {
	t.Helper()
	eligOnce.Do(func() { eligSys, eligErr = zk.SetupEligibility() })
	if eligErr != nil {
		t.Fatalf("eligibility zk setup: %v", eligErr)
	}
	return eligSys
}

func getStakeSystem(t *testing.T) *zk.StakeSystem {
	t.Helper()
	stakeOnce.Do(func() { stakeSys, stakeErr = zk.SetupStake() })
	if stakeErr != nil {
		t.Fatalf("stake zk setup: %v", stakeErr)
	}
	return stakeSys
}

func getUnstakeSystem(t *testing.T) *zk.UnstakeSystem {
	t.Helper()
	unstakeOnce.Do(func() { unstakeSys, unstakeErr = zk.SetupUnstake() })
	if unstakeErr != nil {
		t.Fatalf("unstake zk setup: %v", unstakeErr)
	}
	return unstakeSys
}

// testNetwork is a real store/chain/pipeline (with the real eligibility
// AND stake trees/roots wired in, exactly as a live validator configures
// them) and a real pkg/query API in front of it — stakewallet.Wallet
// talks to this the same way it would a live node.
type testNetwork struct {
	store    *state.Store
	chn      *chain.Chain
	deps     tx.Deps
	pipeline *tx.Pipeline
	queryURL string

	v1id types.NFTID
	v1pk crypto.DilithiumPublicKey
	v1sk crypto.DilithiumPrivateKey

	attestorPK crypto.DilithiumPublicKey
	attestorSK crypto.DilithiumPrivateKey
}

func newTestNetwork(t *testing.T) *testNetwork {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("stakewallet-test-key-32-byte-pad"))
	store, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	eligTree := zk.NewTree()
	initialEligRoot, err := eligTree.Root()
	if err != nil {
		t.Fatalf("initial eligibility root: %v", err)
	}
	zkTree := zk.NewTree()
	initialZKRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial zk root: %v", err)
	}
	stakeTree := zk.NewTree()
	initialStakeRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("initial stake root: %v", err)
	}
	attestorPK, attestorSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen attestor: %v", err)
	}
	deps := tx.Deps{
		Store:               store,
		StateTree:           state.NewMerkleTree(),
		ZKTree:              zkTree,
		ZKRoots:             zk.NewRootHistory(initialZKRoot),
		TrustedPoHAttestors: []crypto.DilithiumPublicKey{attestorPK},
		EligibilityZK:       getEligibilitySystem(t),
		EligibilityTree:     eligTree,
		EligibilityRoots:    zk.NewRootHistory(initialEligRoot),
		StakeZK:             getStakeSystem(t),
		UnstakeZK:           getUnstakeSystem(t),
		StakeTree:           stakeTree,
		StakeRoots:          zk.NewRootHistory(initialStakeRoot),
	}
	pipeline := tx.NewPipeline(deps)

	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen v1: %v", err)
	}

	srv := query.NewServer(store, chn, tx.NewMempool(), query.Config{ListenAddr: "127.0.0.1:0", Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)

	return &testNetwork{
		store: store, chn: chn, deps: deps, pipeline: pipeline,
		queryURL: "http://" + srv.Addr(),
		v1id:     types.NFTID(types.SumHash(v1pk)),
		v1pk:     v1pk, v1sk: v1sk,
		attestorPK: attestorPK, attestorSK: attestorSK,
	}
}

// commit runs txn through the real pipeline and, if accepted, commits a
// real block for it with a genuine single-validator BFT quorum.
func (n *testNetwork) commit(t *testing.T, txn types.ShieldedTx) uint64 {
	t.Helper()
	return n.commitWithPipeline(t, n.pipeline, txn)
}

// commitAtEpoch is commit's counterpart for a transaction that must be
// processed against a later real current epoch than this network's own
// deps.Epoch (0, fixed at construction) — a fresh pipeline sharing every
// other real dependency (Store, trees, ZK systems), only Epoch differs,
// mirroring exactly how a live validator's own Deps.Epoch advances
// between blocks.
func (n *testNetwork) commitAtEpoch(t *testing.T, txn types.ShieldedTx, epoch types.EpochNumber) uint64 {
	t.Helper()
	d := n.deps
	d.Epoch = epoch
	return n.commitWithPipeline(t, tx.NewPipeline(d), txn)
}

func (n *testNetwork) commitWithPipeline(t *testing.T, pipeline *tx.Pipeline, txn types.ShieldedTx) uint64 {
	t.Helper()
	results := pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
	if results[0].Error != nil {
		t.Fatalf("pipeline rejected transaction: %v", results[0].Error)
	}
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		if id == n.v1id {
			return n.v1pk, true
		}
		return nil, false
	}
	b := n.chn.NextBlock(0, []types.ShieldedTx{txn}, types.Hash{1}, types.Hash{2}, types.Hash{}, n.v1id, time.Now().UnixMilli())
	candidate := types.HashBlock(b)
	sig, err := crypto.DilithiumSign(n.v1sk, candidate[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	b.Votes = []types.Vote{{Validator: n.v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig)}}
	if err := n.chn.Append(b, []types.NFTID{n.v1id}, lookup); err != nil {
		t.Fatalf("append: %v", err)
	}
	return b.Height
}

// mintNFT builds and commits a real Kind NFTMint for a fresh identity.
func (n *testNetwork) mintNFT(t *testing.T, nonce uint64) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey) {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	owner := types.AddressFromPubkey(pk)
	att, err := nft.SignPoHAttestation(n.attestorPK, n.attestorSK, owner, nonce, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	voterSK := zk.DeriveVoterSK(sk)
	mintTx := types.ShieldedTx{
		Kind: types.TxNFTMint,
		NFTMintPublicInputs: &types.NFTMintPublicInputs{
			Owner:                 owner,
			Nonce:                 nonce,
			AttestationIssuedAtMs: att.IssuedAtMs,
			Attestor:              []byte(att.Attestor),
			AttestationSig:        types.DilithiumSig(att.Sig),
			VoterCommitment:       types.Hash(zk.ToBytes32(zk.VoterCommitment(voterSK))),
		},
	}
	mintTx.TxID = types.ComputeTxID(mintTx.Proof, mintTx.Commitments, mintTx.Nullifier)
	sig, err := crypto.DilithiumSign(sk, mintTx.TxID[:])
	if err != nil {
		t.Fatalf("sign mint tx: %v", err)
	}
	mintTx.Sig = types.DilithiumSig(sig)
	mintTx.SignerPubKey = []byte(pk)
	n.commit(t, mintTx)
	return pk, sk
}

// TestWalletSyncFindsAndRedeemsRealStakedPosition drives the full,
// real spec-17.4 staked-yield path end to end through this package's own
// public API: propose a real staked mint, reveal it, tally it directly
// (mirroring cmd/wallet's own end-to-end test — this build's CLI/network
// layer has no tally trigger of its own, see pkg/tx.Pipeline.
// TallyDueProposals' own doc), Sync a fresh stakewallet.Wallet purely
// against the real running network, locate the real position, and build
// a real Unstake transaction the real pipeline accepts.
func TestWalletSyncFindsAndRedeemsRealStakedPosition(t *testing.T) {
	net := newTestNetwork(t)
	pk, sk := net.mintNFT(t, 1)

	gw, err := govwallet.New(sk, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new govwallet: %v", err)
	}
	if err := gw.Sync(context.Background()); err != nil {
		t.Fatalf("govwallet sync: %v", err)
	}
	elig, err := gw.BuildEligibilityProof(getEligibilitySystem(t), types.ID("real-staked-proposal"))
	if err != nil {
		t.Fatalf("build eligibility proof: %v", err)
	}

	throwawayPK, throwawaySK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen throwaway signer: %v", err)
	}
	b := txbuilder.New(throwawayPK, throwawaySK)

	const amount = 15000
	proposeTx, position, err := b.ProposeMintStaked("real-staked-proposal", true, amount, 0, getStakeSystem(t), elig)
	if err != nil {
		t.Fatalf("build propose-mint-staked: %v", err)
	}
	net.commit(t, proposeTx)

	revealTx, err := b.VoteReveal("real-staked-proposal", true, elig)
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	net.commit(t, revealTx)

	if _, err := net.pipeline.TallyDueProposals(1); err != nil {
		t.Fatalf("tally: %v", err)
	}

	// A fresh stakewallet.Wallet, syncing purely against the real
	// running network — no shortcut access to the pipeline's own tree.
	w, err := stakewallet.New(stakewallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new stakewallet: %v", err)
	}
	w.Remember(position)
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("stakewallet sync: %v", err)
	}
	if !w.Found(position) {
		t.Fatalf("expected the wallet to find its own real position after a real passed, tallied proposal")
	}

	root, err := w.CurrentRoot()
	if err != nil {
		t.Fatalf("current root: %v", err)
	}
	proof, err := w.ProofFor(position)
	if err != nil {
		t.Fatalf("proof for: %v", err)
	}

	unstakeSigner := txbuilder.New(pk, sk)
	unstakeTx, out, err := unstakeSigner.Unstake(position, proof, root, 50, getUnstakeSystem(t))
	if err != nil {
		t.Fatalf("build unstake: %v", err)
	}
	if out.Value <= amount {
		t.Fatalf("expected the real redeemed note to carry principal plus some real positive yield, got %d for principal %d", out.Value, amount)
	}
	net.commitAtEpoch(t, unstakeTx, 50)
}

func TestWalletFoundIsFalseBeforeSync(t *testing.T) {
	w, err := stakewallet.New(stakewallet.Config{QueryBase: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("new stakewallet: %v", err)
	}
	ownerSK, _ := zk.NewSpendKey()
	rho, _ := zk.NewRho()
	secret := zk.StakeSecret{Principal: 100, StartEpoch: 0, OwnerSK: ownerSK, Rho: rho}
	w.Remember(secret)
	if w.Found(secret) {
		t.Fatalf("expected Found to be false before any Sync has run")
	}
	if _, err := w.ProofFor(secret); err == nil {
		t.Fatalf("expected ProofFor to fail before any Sync has run")
	}
}
