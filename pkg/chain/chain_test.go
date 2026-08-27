package chain_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func openTestStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("chain-test-key-32-bytes-padding!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// validatorKeys generates n real Dilithium keypairs, keyed by NFTID
// (derived from the public key, mirroring how pkg/validator will derive
// consensus identities from real signing keys).
type validatorKeys struct {
	ids []types.NFTID
	pk  map[types.NFTID]crypto.DilithiumPublicKey
	sk  map[types.NFTID]crypto.DilithiumPrivateKey
}

func genValidators(t *testing.T, n int) *validatorKeys {
	t.Helper()
	v := &validatorKeys{pk: map[types.NFTID]crypto.DilithiumPublicKey{}, sk: map[types.NFTID]crypto.DilithiumPrivateKey{}}
	for i := 0; i < n; i++ {
		pk, sk, err := crypto.GenerateDilithiumKey()
		if err != nil {
			t.Fatalf("generate key %d: %v", i, err)
		}
		id := types.NFTID(types.SumHash(pk))
		v.ids = append(v.ids, id)
		v.pk[id] = pk
		v.sk[id] = sk
	}
	return v
}

func (v *validatorKeys) lookup(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
	pk, ok := v.pk[id]
	return pk, ok
}

func (v *validatorKeys) sign(id types.NFTID, msg []byte) types.DilithiumSig {
	sig, err := crypto.DilithiumSign(v.sk[id], msg)
	if err != nil {
		panic(err)
	}
	return types.DilithiumSig(sig)
}

func TestOpenCreatesGenesis(t *testing.T) {
	s := openTestStore(t)
	c, err := chain.Open(s, 12345)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if c.HeadHeight() != 0 {
		t.Fatalf("expected genesis height 0, got %d", c.HeadHeight())
	}
	b, found, err := s.GetBlock(0)
	if err != nil || !found {
		t.Fatalf("expected genesis block persisted: found=%v err=%v", found, err)
	}
	if b.Timestamp != 12345 {
		t.Fatalf("unexpected genesis timestamp: %d", b.Timestamp)
	}
}

func TestOpenReloadsExistingHead(t *testing.T) {
	s := openTestStore(t)
	c1, err := chain.Open(s, 1)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	v := genValidators(t, 3)
	b := c1.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 2)
	candidate := types.HashBlock(b)
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: v.ids[1], StateRoot: candidate, Sig: v.sign(v.ids[1], candidate[:])},
	}
	if err := c1.Append(b, v.ids, v.lookup); err != nil {
		t.Fatalf("append: %v", err)
	}

	c2, err := chain.Open(s, 1) // re-open against the same store
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if c2.HeadHeight() != 1 {
		t.Fatalf("expected reloaded head height 1, got %d", c2.HeadHeight())
	}
	if c2.HeadHash() != c1.HeadHash() {
		t.Fatalf("reloaded head hash does not match")
	}
}

func TestAppendWithRealQuorumSucceeds(t *testing.T) {
	s := openTestStore(t)
	c, err := chain.Open(s, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	v := genValidators(t, 5) // majority of 5 is 3

	b := c.NextBlock(0, nil, types.Hash{9}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	candidate := types.HashBlock(b)
	for i := 0; i < 3; i++ {
		b.Votes = append(b.Votes, types.Vote{Validator: v.ids[i], StateRoot: candidate, Sig: v.sign(v.ids[i], candidate[:])})
	}

	if err := c.Append(b, v.ids, v.lookup); err != nil {
		t.Fatalf("expected append with real 3/5 quorum to succeed: %v", err)
	}
	if c.HeadHeight() != 1 {
		t.Fatalf("expected head height 1, got %d", c.HeadHeight())
	}
	if c.HeadHash() != candidate {
		t.Fatalf("expected head hash to be the committed block's hash")
	}
}

func TestAppendRejectsInsufficientVotes(t *testing.T) {
	s := openTestStore(t)
	c, _ := chain.Open(s, 0)
	v := genValidators(t, 5)

	b := c.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	candidate := types.HashBlock(b)
	// Only 2 of 5 sign — below majority.
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: v.ids[1], StateRoot: candidate, Sig: v.sign(v.ids[1], candidate[:])},
	}
	if err := c.Append(b, v.ids, v.lookup); err == nil {
		t.Fatalf("expected append to fail with only 2/5 votes")
	}
	if c.HeadHeight() != 0 {
		t.Fatalf("head must not advance on a rejected append")
	}
}

func TestAppendRejectsForgedSignature(t *testing.T) {
	s := openTestStore(t)
	c, _ := chain.Open(s, 0)
	v := genValidators(t, 5)
	attacker := genValidators(t, 1) // a key not in the committee at all

	b := c.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	candidate := types.HashBlock(b)
	// 3 votes, but 2 of them are forged: signed by a key that isn't the
	// claimed validator (claims to be v.ids[1]/v.ids[2] but is actually
	// signed by the attacker's key).
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: v.ids[1], StateRoot: candidate, Sig: attacker.sign(attacker.ids[0], candidate[:])},
		{Validator: v.ids[2], StateRoot: candidate, Sig: attacker.sign(attacker.ids[0], candidate[:])},
	}
	if err := c.Append(b, v.ids, v.lookup); err == nil {
		t.Fatalf("expected append to reject a block whose votes don't actually verify")
	}
}

func TestAppendRejectsNonCommitteeSigner(t *testing.T) {
	s := openTestStore(t)
	c, _ := chain.Open(s, 0)
	v := genValidators(t, 2)
	outsider := genValidators(t, 3) // real, valid signatures — just not committee members

	b := c.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	candidate := types.HashBlock(b)
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: outsider.ids[0], StateRoot: candidate, Sig: outsider.sign(outsider.ids[0], candidate[:])},
		{Validator: outsider.ids[1], StateRoot: candidate, Sig: outsider.sign(outsider.ids[1], candidate[:])},
	}
	// Committee is only v (2 members); lookup must resolve everyone so we
	// merge both keyrings, but committee membership should still exclude
	// the outsiders.
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		if pk, ok := v.lookup(id); ok {
			return pk, ok
		}
		return outsider.lookup(id)
	}
	if err := c.Append(b, v.ids, lookup); err == nil {
		t.Fatalf("expected append to fail: only 1 of 2 committee members voted, outsiders don't count")
	}
}

func TestAppendRejectsWrongPrevHash(t *testing.T) {
	s := openTestStore(t)
	c, _ := chain.Open(s, 0)
	v := genValidators(t, 3)

	b := c.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	b.PrevHash = types.Hash{0xFF} // tamper
	candidate := types.HashBlock(b)
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: v.ids[1], StateRoot: candidate, Sig: v.sign(v.ids[1], candidate[:])},
	}
	if err := c.Append(b, v.ids, v.lookup); err == nil {
		t.Fatalf("expected append to reject a block with the wrong PrevHash")
	}
}

func TestAppendRejectsWrongHeight(t *testing.T) {
	s := openTestStore(t)
	c, _ := chain.Open(s, 0)
	v := genValidators(t, 3)

	b := c.NextBlock(0, nil, types.Hash{}, types.Hash{1}, types.Hash{}, v.ids[0], 100)
	b.Height = 5 // skip ahead
	candidate := types.HashBlock(b)
	b.Votes = []types.Vote{
		{Validator: v.ids[0], StateRoot: candidate, Sig: v.sign(v.ids[0], candidate[:])},
		{Validator: v.ids[1], StateRoot: candidate, Sig: v.sign(v.ids[1], candidate[:])},
	}
	if err := c.Append(b, v.ids, v.lookup); err == nil {
		t.Fatalf("expected append to reject a block that skips a height")
	}
}
