package govwallet_test

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
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	eligOnce sync.Once
	eligSys  *zk.EligibilitySystem
	eligErr  error
)

func getEligibilitySystem(t *testing.T) *zk.EligibilitySystem {
	t.Helper()
	eligOnce.Do(func() { eligSys, eligErr = zk.SetupEligibility() })
	if eligErr != nil {
		t.Fatalf("eligibility zk setup: %v", eligErr)
	}
	return eligSys
}

func openStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("govwallet-test-key-32-bytes-pad!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// testNetwork is a real store/chain/pipeline (with the real eligibility
// tree/roots wired in, exactly as a live validator configures it) and a
// real pkg/query API in front of it — govwallet.Wallet talks to this the
// same way it would a live node.
type testNetwork struct {
	store    *state.Store
	chn      *chain.Chain
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
	store := openStore(t)
	chn, err := chain.Open(store, 1)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	eligTree := zk.NewTree()
	initialRoot, err := eligTree.Root()
	if err != nil {
		t.Fatalf("initial eligibility root: %v", err)
	}
	attestorPK, attestorSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen attestor: %v", err)
	}
	pipeline := tx.NewPipeline(tx.Deps{
		Store:               store,
		StateTree:           state.NewMerkleTree(),
		TrustedPoHAttestors: []crypto.DilithiumPublicKey{attestorPK},
		EligibilityZK:       getEligibilitySystem(t),
		EligibilityTree:     eligTree,
		EligibilityRoots:    zk.NewRootHistory(initialRoot),
	})

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
		store: store, chn: chn, pipeline: pipeline,
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
	results := n.pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
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

// mintNFT builds and commits a real Kind NFTMint for a fresh identity,
// returning that identity's real secret key — the same real path
// cmd/wallet's "nft-mint" command drives, done here directly for test
// speed.
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

func TestWalletSyncFindsOwnVoterCommitmentAndBuildsValidProof(t *testing.T) {
	net := newTestNetwork(t)
	_, sk := net.mintNFT(t, 1)

	w, err := govwallet.New(sk, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !w.Eligible() {
		t.Fatalf("expected the wallet to find its own real VoterCommitment after syncing a real committed NFTMint")
	}

	proof, err := w.BuildEligibilityProof(getEligibilitySystem(t), types.ID("real-proposal"))
	if err != nil {
		t.Fatalf("build eligibility proof: %v", err)
	}
	if len(proof.Proof) == 0 {
		t.Fatalf("expected real, non-empty proof bytes")
	}

	// The real, independent verification path: exactly what pkg/tx's
	// pipeline (requireEligibleVoterZK) does with a submitted proof.
	rootElem := zk.FieldElementFromBytes32(proof.MerkleRoot)
	nullifierElem := zk.FieldElementFromBytes32(proof.Nullifier)
	scopeElem := zk.FieldElementFromBytes32(types.VoteEligibilityScope(types.ID("real-proposal")))
	pub := zk.EligibilityPublic{MerkleRoot: rootElem, Nullifier: nullifierElem, ProposalScope: scopeElem}
	if err := getEligibilitySystem(t).VerifyPublicProofBytes(proof.Proof, pub); err != nil {
		t.Fatalf("expected a real wallet-built proof to verify: %v", err)
	}
}

func TestWalletBuildEligibilityProofFailsWithoutAnyMint(t *testing.T) {
	net := newTestNetwork(t)
	_, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	w, err := govwallet.New(sk, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if w.Eligible() {
		t.Fatalf("expected a wallet that never minted an NFT to not be eligible")
	}
	if _, err := w.BuildEligibilityProof(getEligibilitySystem(t), types.ID("real-proposal")); err == nil {
		t.Fatalf("expected building a proof with no known VoterCommitment to fail")
	}
}

// TestWalletProofsForDifferentProposalsAreUnlinkable proves the real
// anonymity/unlinkability property spec 9.1's anonymous eligibility
// design depends on: the same real minted NFT produces a DIFFERENT
// nullifier for two different proposals, so an observer of both votes
// cannot tell they came from the same voter — while a proof for one
// proposal never verifies against the other's scope.
func TestWalletProofsForDifferentProposalsAreUnlinkable(t *testing.T) {
	net := newTestNetwork(t)
	_, sk := net.mintNFT(t, 1)

	w, err := govwallet.New(sk, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	sys := getEligibilitySystem(t)
	proofA, err := w.BuildEligibilityProof(sys, types.ID("proposal-a"))
	if err != nil {
		t.Fatalf("build proof A: %v", err)
	}
	proofB, err := w.BuildEligibilityProof(sys, types.ID("proposal-b"))
	if err != nil {
		t.Fatalf("build proof B: %v", err)
	}
	if proofA.Nullifier == proofB.Nullifier {
		t.Fatalf("expected distinct nullifiers for two different proposals from the same real voter")
	}

	// proofA must not verify against proposal-b's scope.
	rootElem := zk.FieldElementFromBytes32(proofA.MerkleRoot)
	nullifierElem := zk.FieldElementFromBytes32(proofA.Nullifier)
	wrongScope := zk.FieldElementFromBytes32(types.VoteEligibilityScope(types.ID("proposal-b")))
	pub := zk.EligibilityPublic{MerkleRoot: rootElem, Nullifier: nullifierElem, ProposalScope: wrongScope}
	if err := sys.VerifyPublicProofBytes(proofA.Proof, pub); err == nil {
		t.Fatalf("expected proposal-a's proof to fail verification against proposal-b's scope")
	}
}

// TestWalletDistinctVotersProduceDistinctNullifiersSameProposal proves
// two different real minted NFTs voting on the SAME proposal get
// distinct nullifiers (so both ballots count, neither collides with the
// other) while each individually still verifies.
func TestWalletDistinctVotersProduceDistinctNullifiersSameProposal(t *testing.T) {
	net := newTestNetwork(t)
	_, sk1 := net.mintNFT(t, 1)
	_, sk2 := net.mintNFT(t, 2)

	w1, err := govwallet.New(sk1, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet 1: %v", err)
	}
	w2, err := govwallet.New(sk2, govwallet.Config{QueryBase: net.queryURL})
	if err != nil {
		t.Fatalf("new wallet 2: %v", err)
	}
	if err := w1.Sync(context.Background()); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	if err := w2.Sync(context.Background()); err != nil {
		t.Fatalf("sync 2: %v", err)
	}

	sys := getEligibilitySystem(t)
	proof1, err := w1.BuildEligibilityProof(sys, types.ID("shared-proposal"))
	if err != nil {
		t.Fatalf("build proof 1: %v", err)
	}
	proof2, err := w2.BuildEligibilityProof(sys, types.ID("shared-proposal"))
	if err != nil {
		t.Fatalf("build proof 2: %v", err)
	}
	if proof1.Nullifier == proof2.Nullifier {
		t.Fatalf("expected distinct nullifiers for two distinct real voters on the same proposal")
	}

	scopeElem := zk.FieldElementFromBytes32(types.VoteEligibilityScope(types.ID("shared-proposal")))
	for i, proof := range []types.VoteEligibilityProof{proof1, proof2} {
		pub := zk.EligibilityPublic{
			MerkleRoot:    zk.FieldElementFromBytes32(proof.MerkleRoot),
			Nullifier:     zk.FieldElementFromBytes32(proof.Nullifier),
			ProposalScope: scopeElem,
		}
		if err := sys.VerifyPublicProofBytes(proof.Proof, pub); err != nil {
			t.Fatalf("expected voter %d's proof to verify: %v", i+1, err)
		}
	}
}
