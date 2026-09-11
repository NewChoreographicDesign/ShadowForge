package validator

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
)

// newGatedTestNode builds one real validator.Node with direct control over
// the real heartbeat-admission gate this file tests (skipNFTCheck,
// trustedSentinelKeys) — newTestNode elsewhere in this package always
// passes skipNFTCheck=true (existing consensus-mechanics tests don't need
// real NFTs), so these tests need their own constructor to exercise the
// gate itself, plus direct access to the real *state.Store to seed (or
// deliberately not seed) real ValidatorNFT records.
func newGatedTestNode(t *testing.T, skipNFTCheck bool, trustedSentinelKeys []crypto.DilithiumPublicKey) (*Node, *state.Store) {
	t.Helper()
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	store := openTestStore(t)
	tree := state.NewMerkleTree()
	genesisMs := time.Now().UnixMilli()
	chn, err := chain.Open(store, genesisMs)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	v := vault.New(vault.DefaultSplits())
	mempool := tx.NewMempool()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	cfg := Config{
		BatchInterval:     time.Second,
		RoundTimeout:      2 * time.Second,
		HeartbeatInterval: consensus.HeartbeatInterval,
		OnlineTimeout:     time.Minute,
		Genesis:           consensus.GenesisTime(genesisMs),
	}
	n := NewNode(cfg, h, nil, store, tree, chn, nil, v, nil, nil, getTestEligibilitySystem(t), getTestMintSystem(t), getTestStakeSystem(t), getTestUnstakeSystem(t), mempool, pk, sk, false, skipNFTCheck, trustedSentinelKeys, testLogf(t))
	return n, store
}

// signedHeartbeat builds a real HeartbeatPayload envelope signed by sk over
// PubKey/Timestamp/IsSentinel — mirroring sendHeartbeat's own real signing
// exactly, so these tests exercise the genuine wire format a real peer's
// handleMessage would receive, not a simplified stand-in for it.
func signedHeartbeat(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, isSentinel bool) shadownet.Envelope {
	t.Helper()
	now := time.Now().UnixMilli()
	msg := shadownet.HeartbeatMessage([]byte(pk), now, isSentinel)
	sig, err := crypto.DilithiumSign(sk, msg[:])
	if err != nil {
		t.Fatalf("sign heartbeat: %v", err)
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
		PubKey: []byte(pk), Timestamp: now, Sig: types.DilithiumSig(sig), IsSentinel: isSentinel,
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	return env
}

// seedRealNFT stores a real ValidatorNFT owned by AddressFromPubkey(pk) —
// the exact real lookup key the heartbeat-admission gate queries
// (state.Store.GetNFTByOwner), so a test seeding one here is indistinguishable
// from that identity having genuinely minted one via the real pkg/nft.Mint
// flow.
func seedRealNFT(t *testing.T, store *state.Store, pk crypto.DilithiumPublicKey, slashed bool) {
	t.Helper()
	owner := types.AddressFromPubkey(pk)
	nft := types.ValidatorNFT{
		ID:      types.NFTID(types.SumHash(owner[:], []byte("nonce"))),
		Owner:   owner,
		Traits:  map[string]string{},
		Slashed: slashed,
	}
	if err := store.PutNFT(nft); err != nil {
		t.Fatalf("seed real NFT: %v", err)
	}
}

const testPeer = peer.ID("test-peer")

func TestHeartbeatRejectedWithoutValidSignature(t *testing.T) {
	n, _ := newGatedTestNode(t, false, nil)
	before := n.OnlineValidatorCount(time.Now())

	otherPK, otherSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	// A genuine signature, but over the WRONG claimed pubkey (signed by a
	// second, unrelated key) — exactly what an attacker forging someone
	// else's identity would produce: a structurally valid signature that
	// simply does not verify against the PubKey the heartbeat claims.
	impostorPK, _, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate impostor identity: %v", err)
	}
	now := time.Now().UnixMilli()
	msg := shadownet.HeartbeatMessage([]byte(impostorPK), now, false)
	sig, err := crypto.DilithiumSign(otherSK, msg[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgHeartbeat, shadownet.HeartbeatPayload{
		PubKey: []byte(impostorPK), Timestamp: now, Sig: types.DilithiumSig(sig),
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	n.handleMessage(testPeer, env)

	if got := n.OnlineValidatorCount(time.Now()); got != before {
		t.Fatalf("expected an unsigned/forged heartbeat to be rejected (count unchanged at %d), got %d", before, got)
	}
	_ = otherPK
}

func TestHeartbeatFromRealNFTHolderIsAdmitted(t *testing.T) {
	n, store := newGatedTestNode(t, false, nil)
	before := n.OnlineValidatorCount(time.Now())

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	seedRealNFT(t, store, pk, false)

	n.handleMessage(testPeer, signedHeartbeat(t, pk, sk, false))

	if got := n.OnlineValidatorCount(time.Now()); got != before+1 {
		t.Fatalf("expected a real NFT holder's genuine heartbeat to be admitted (count %d), got %d", before+1, got)
	}
}

func TestHeartbeatFromNonNFTHolderIsRejected(t *testing.T) {
	n, _ := newGatedTestNode(t, false, nil)
	before := n.OnlineValidatorCount(time.Now())

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	// Deliberately never seeded with a real NFT.

	n.handleMessage(testPeer, signedHeartbeat(t, pk, sk, false))

	if got := n.OnlineValidatorCount(time.Now()); got != before {
		t.Fatalf("expected a genuinely-signed heartbeat from an identity with no real NFT to be rejected (count unchanged at %d), got %d", before, got)
	}
}

func TestHeartbeatFromSlashedNFTHolderIsRejected(t *testing.T) {
	n, store := newGatedTestNode(t, false, nil)
	before := n.OnlineValidatorCount(time.Now())

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	seedRealNFT(t, store, pk, true) // slashed

	n.handleMessage(testPeer, signedHeartbeat(t, pk, sk, false))

	if got := n.OnlineValidatorCount(time.Now()); got != before {
		t.Fatalf("expected a slashed NFT holder's heartbeat to be rejected (count unchanged at %d), got %d", before, got)
	}
}

func TestUntrustedSentinelClaimFallsThroughToNFTCheck(t *testing.T) {
	// No trustedSentinelKeys configured at all: nobody's IsSentinel claim
	// can ever be honored.
	n, _ := newGatedTestNode(t, false, nil)
	before := n.OnlineValidatorCount(time.Now())

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	// No real NFT seeded either — if the IsSentinel claim were wrongly
	// honored, this would be admitted for free; it must not be.

	n.handleMessage(testPeer, signedHeartbeat(t, pk, sk, true))

	if got := n.OnlineValidatorCount(time.Now()); got != before {
		t.Fatalf("expected an untrusted IsSentinel claim to fall through to the real NFT gate and be rejected (count unchanged at %d), got %d", before, got)
	}
}

func TestHeartbeatFromTrustedSentinelIsAdmittedWithoutNFT(t *testing.T) {
	sentinelPK, sentinelSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate sentinel identity: %v", err)
	}
	n, _ := newGatedTestNode(t, false, []crypto.DilithiumPublicKey{sentinelPK})
	before := n.OnlineValidatorCount(time.Now())

	// No NFT seeded for the sentinel — a real trusted sentinel never needs
	// one.
	n.handleMessage(testPeer, signedHeartbeat(t, sentinelPK, sentinelSK, true))

	if got := n.OnlineValidatorCount(time.Now()); got != before+1 {
		t.Fatalf("expected a real trusted sentinel's heartbeat to be admitted without any NFT (count %d), got %d", before+1, got)
	}
}

func TestSkipNFTCheckBypassesGate(t *testing.T) {
	n, _ := newGatedTestNode(t, true, nil) // skipNFTCheck=true, the local-dev escape hatch
	before := n.OnlineValidatorCount(time.Now())

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	// No NFT seeded — with the real gate skipped, this must still be
	// admitted purely on a valid signature, matching every pre-existing
	// consensus-mechanics test in this package that relies on exactly
	// this escape hatch.

	n.handleMessage(testPeer, signedHeartbeat(t, pk, sk, false))

	if got := n.OnlineValidatorCount(time.Now()); got != before+1 {
		t.Fatalf("expected -skip-nft-check to admit a signed heartbeat with no real NFT (count %d), got %d", before+1, got)
	}
}
