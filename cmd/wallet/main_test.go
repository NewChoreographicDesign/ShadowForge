package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/query"
	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// TestReadPassphraseLineReadsSequentialLines pins the same real bug fix
// cmd/walletkey's identical test guards — a fresh bufio.Scanner per call
// silently loses whatever a prior call's scanner had already buffered
// past the first newline.
func TestReadPassphraseLineReadsSequentialLines(t *testing.T) {
	stdinSource = strings.NewReader("old-line\nnew-line\n")
	stdinScanner = nil
	t.Cleanup(func() { stdinScanner = nil })

	first, err := readPassphraseLine()
	if err != nil || first != "old-line" {
		t.Fatalf("first line: got %q, err %v", first, err)
	}
	second, err := readPassphraseLine()
	if err != nil || second != "new-line" {
		t.Fatalf("second line: got %q, err %v", second, err)
	}
}

func withStdin(t *testing.T, lines ...string) {
	t.Helper()
	stdinSource = strings.NewReader(strings.Join(lines, "\n") + "\n")
	stdinScanner = nil
	t.Cleanup(func() { stdinScanner = nil })
}

// captureStdout runs f with os.Stdout redirected to a pipe and returns
// whatever it printed — real output, not a mocked writer, since every
// runXxx function in this package prints straight to os.Stdout via fmt.
func captureStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	runErr := f()
	os.Stdout = orig
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), runErr
}

func newTestKeystore(t *testing.T, passphrase string) (path string, ks *walletkey.Keystore) {
	t.Helper()
	ks, err := walletkey.Generate(passphrase)
	if err != nil {
		t.Fatalf("generate keystore: %v", err)
	}
	f, err := os.CreateTemp(t.TempDir(), "walletkey-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	path = f.Name()
	_ = f.Close()
	if err := ks.Save(path); err != nil {
		t.Fatalf("save keystore: %v", err)
	}
	return path, ks
}

// --- identity (offline) ---

func TestCLIIdentity(t *testing.T) {
	path, ks := newTestKeystore(t, "correct horse battery staple")
	out, err := captureStdout(t, func() error {
		return runIdentity([]string{"-keystore", path})
	})
	if err != nil {
		t.Fatalf("runIdentity: %v", err)
	}
	if !strings.Contains(out, ks.Identity().String()) {
		t.Fatalf("expected output to contain identity %s, got:\n%s", ks.Identity(), out)
	}
	if !strings.Contains(out, hex.EncodeToString(ks.ShieldedPublicKey().Bytes())) {
		t.Fatalf("expected output to contain shielded public key, got:\n%s", out)
	}
}

// --- read-only queries, against a real query.Server ---

type queryTestEnv struct {
	store *state.Store
	chn   *chain.Chain
	base  string
}

func newQueryTestEnv(t *testing.T) *queryTestEnv {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("cmd-wallet-query-test-key-32-by!"))
	store, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	chn, err := chain.Open(store, 1735689600000)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	srv := query.NewServer(store, chn, tx.NewMempool(), query.Config{ListenAddr: "127.0.0.1:0", Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)
	return &queryTestEnv{store: store, chn: chn, base: "http://" + srv.Addr()}
}

func TestCLIStatus(t *testing.T) {
	env := newQueryTestEnv(t)
	out, err := captureStdout(t, func() error {
		return runStatus([]string{"-query", env.base})
	})
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	if !strings.Contains(out, "height: 0") {
		t.Fatalf("expected height 0, got:\n%s", out)
	}
}

func TestCLITxStatusUnknown(t *testing.T) {
	env := newQueryTestEnv(t)
	out, err := captureStdout(t, func() error {
		return runTxStatus([]string{"-query", env.base, "-txid", strings.Repeat("00", 32)})
	})
	if err != nil {
		t.Fatalf("runTxStatus: %v", err)
	}
	if !strings.Contains(out, "status: unknown") {
		t.Fatalf("expected unknown status, got:\n%s", out)
	}
}

func TestCLINullifierAndNote(t *testing.T) {
	env := newQueryTestEnv(t)
	n := types.Hash{0x11}
	if err := env.store.MarkNullifierSpent(n); err != nil {
		t.Fatalf("mark spent: %v", err)
	}
	out, err := captureStdout(t, func() error {
		return runNullifier([]string{"-query", env.base, "-hash", n.String()})
	})
	if err != nil {
		t.Fatalf("runNullifier: %v", err)
	}
	if !strings.Contains(out, "spent: true") {
		t.Fatalf("expected spent: true, got:\n%s", out)
	}

	note := types.Note{Commitment: types.Hash{0x22}, Value: 5, Asset: "SFG"}
	if err := env.store.PutNote(note); err != nil {
		t.Fatalf("put note: %v", err)
	}
	out, err = captureStdout(t, func() error {
		return runNote([]string{"-query", env.base, "-commitment", note.Commitment.String()})
	})
	if err != nil {
		t.Fatalf("runNote: %v", err)
	}
	if !strings.Contains(out, "exists: true") {
		t.Fatalf("expected exists: true, got:\n%s", out)
	}
}

func TestCLINFTAndHold(t *testing.T) {
	env := newQueryTestEnv(t)
	nft := types.ValidatorNFT{ID: types.NFTID{0x33}, Owner: types.Address{0x01}, TP: 7}
	if err := env.store.PutNFT(nft); err != nil {
		t.Fatalf("put nft: %v", err)
	}
	out, err := captureStdout(t, func() error {
		return runNFT([]string{"-query", env.base, "-id", nft.ID.String()})
	})
	if err != nil {
		t.Fatalf("runNFT: %v", err)
	}
	if !strings.Contains(out, "tp: 7") {
		t.Fatalf("expected tp: 7, got:\n%s", out)
	}

	if err := runNFT([]string{"-query", env.base, "-id", strings.Repeat("ff", 32)}); err == nil {
		t.Fatalf("expected an error for an unminted NFT id")
	}

	hold := types.BankHold{HoldID: types.Hash{0x44}, SFGIssued: 99}
	if err := env.store.PutHold(hold); err != nil {
		t.Fatalf("put hold: %v", err)
	}
	out, err = captureStdout(t, func() error {
		return runHold([]string{"-query", env.base, "-id", hold.HoldID.String()})
	})
	if err != nil {
		t.Fatalf("runHold: %v", err)
	}
	if !strings.Contains(out, "sfg issued: 99") {
		t.Fatalf("expected sfg issued: 99, got:\n%s", out)
	}
}

func TestCLIProposals(t *testing.T) {
	env := newQueryTestEnv(t)
	if err := env.store.PutProposal(state.ProposalRecord{ProposalID: "prop-x", Epoch: 2, Approve: 3, Reject: 1, Passed: true, Tallied: true}); err != nil {
		t.Fatalf("put proposal: %v", err)
	}
	out, err := captureStdout(t, func() error {
		return runProposal([]string{"-query", env.base, "-id", "prop-x"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "approve: 3") || !strings.Contains(out, "passed: true") {
		t.Fatalf("unexpected proposal output:\n%s", out)
	}

	out, err = captureStdout(t, func() error {
		return runProposals([]string{"-query", env.base})
	})
	if err != nil {
		t.Fatalf("runProposals: %v", err)
	}
	if !strings.Contains(out, "prop-x") {
		t.Fatalf("expected proposals list to include prop-x, got:\n%s", out)
	}
}

// --- a real, single-validator live "network" for submit-path tests ---
//
// testBackend is a real libp2p peer this binary's own submitTx connects
// and broadcasts to: on receiving a real TxOffer it runs the transaction
// through a real tx.Pipeline and, if accepted, commits it via a real
// single-validator BFT quorum (chain.Append with a one-member committee —
// the same lone-validator self-quorum this codebase's consensus package
// deliberately supports for cold-start, not a test shortcut). This
// proves cmd/wallet's actual network.go code — connect, broadcast,
// confirm via pkg/query — against genuine wire traffic, without
// re-exercising pkg/validator's own full multi-node BFT round machinery
// (already covered by that package's own tests).
type testBackend struct {
	store    *state.Store
	chn      *chain.Chain
	deps     tx.Deps
	pipeline *tx.Pipeline
	queryURL string
	addr     string

	v1id types.NFTID
	v1pk crypto.DilithiumPublicKey
	v1sk crypto.DilithiumPrivateKey
	logf func(format string, args ...any)

	// attestorKeystorePath is a real, saved walletkey.Keystore this
	// backend's pipeline trusts as a proof-of-humanity attestor — tests
	// mint real NFTs through the actual 'wallet poh-attest'/'wallet
	// nft-mint' CLI commands using it, before exercising anything that
	// now needs real voter eligibility.
	attestorKeystorePath       string
	attestorKeystorePassphrase string

	// eligibilityZKParamsPath is a real, shared Groth16 params file for
	// the anonymous voter-eligibility circuit, written from the exact
	// same in-process zk.EligibilitySystem this backend's pipeline
	// verifies against — see 'wallet vote'/'wallet vote-reveal' own doc
	// on why the CLI must load real, shared params from a file rather
	// than run its own independent setup.
	eligibilityZKParamsPath string
	// mintZKParamsPath is eligibilityZKParamsPath's counterpart for the
	// real spec-17.4 epoch-mint circuit — see 'wallet propose-mint' own
	// doc.
	mintZKParamsPath string
	// stakeZKParamsPath/unstakeZKParamsPath are mintZKParamsPath's
	// counterparts for the real spec-17.4 staked-yield mint path — see
	// 'wallet propose-mint -staked'/'wallet unstake' own docs.
	stakeZKParamsPath   string
	unstakeZKParamsPath string
}

func newTestBackend(t *testing.T, storeKeyByte byte, zkSys *zk.System, zkTree *zk.Tree, zkRoots *zk.RootHistory) *testBackend {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("cmd-wallet-backend-test-key-32b!"))
	key[31] = storeKeyByte
	store, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	genesisMs := time.Now().UnixMilli()
	chn, err := chain.Open(store, genesisMs)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	attestorPath, attestorKS := newTestKeystore(t, "attestor-passphrase")

	// Real, shared anonymous-voter-eligibility params — every Vote/
	// VoteReveal-exercising test in this file needs this backend's
	// pipeline and the CLI's own loaded copy (from the file this writes)
	// to verify against identical Groth16 keys, exactly like zkSys above
	// for Kind Transfer.
	eligSys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("eligibility zk setup: %v", err)
	}
	eligParamsPath := filepath.Join(t.TempDir(), "eligibility-zk-params.bin")
	eligParamsFile, err := os.Create(eligParamsPath)
	if err != nil {
		t.Fatalf("create eligibility zk params file: %v", err)
	}
	if _, err := eligSys.WriteTo(eligParamsFile); err != nil {
		t.Fatalf("write eligibility zk params: %v", err)
	}
	if err := eligParamsFile.Close(); err != nil {
		t.Fatalf("close eligibility zk params file: %v", err)
	}
	eligTree := zk.NewTree()
	initialEligRoot, err := eligTree.Root()
	if err != nil {
		t.Fatalf("initial eligibility root: %v", err)
	}

	// Real, shared epoch-mint params — every propose-mint-exercising
	// test in this file needs this backend's pipeline and the CLI's own
	// loaded copy to verify against identical Groth16 keys, exactly like
	// eligSys above.
	mintSys, err := zk.SetupMint()
	if err != nil {
		t.Fatalf("mint zk setup: %v", err)
	}
	mintParamsPath := filepath.Join(t.TempDir(), "mint-zk-params.bin")
	mintParamsFile, err := os.Create(mintParamsPath)
	if err != nil {
		t.Fatalf("create mint zk params file: %v", err)
	}
	if _, err := mintSys.WriteTo(mintParamsFile); err != nil {
		t.Fatalf("write mint zk params: %v", err)
	}
	if err := mintParamsFile.Close(); err != nil {
		t.Fatalf("close mint zk params file: %v", err)
	}

	// Real, shared staked-yield mint params — every propose-mint
	// -staked/unstake-exercising test in this file needs this backend's
	// pipeline and the CLI's own loaded copy to verify against identical
	// Groth16 keys, exactly like mintSys above.
	stakeSys, err := zk.SetupStake()
	if err != nil {
		t.Fatalf("stake zk setup: %v", err)
	}
	stakeParamsPath := filepath.Join(t.TempDir(), "stake-zk-params.bin")
	stakeParamsFile, err := os.Create(stakeParamsPath)
	if err != nil {
		t.Fatalf("create stake zk params file: %v", err)
	}
	if _, err := stakeSys.WriteTo(stakeParamsFile); err != nil {
		t.Fatalf("write stake zk params: %v", err)
	}
	if err := stakeParamsFile.Close(); err != nil {
		t.Fatalf("close stake zk params file: %v", err)
	}
	unstakeSys, err := zk.SetupUnstake()
	if err != nil {
		t.Fatalf("unstake zk setup: %v", err)
	}
	unstakeParamsPath := filepath.Join(t.TempDir(), "unstake-zk-params.bin")
	unstakeParamsFile, err := os.Create(unstakeParamsPath)
	if err != nil {
		t.Fatalf("create unstake zk params file: %v", err)
	}
	if _, err := unstakeSys.WriteTo(unstakeParamsFile); err != nil {
		t.Fatalf("write unstake zk params: %v", err)
	}
	if err := unstakeParamsFile.Close(); err != nil {
		t.Fatalf("close unstake zk params file: %v", err)
	}
	stakeTree := zk.NewTree()
	initialStakeRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("initial stake root: %v", err)
	}

	deps := tx.Deps{
		Store:               store,
		StateTree:           state.NewMerkleTree(),
		ZK:                  zkSys,
		ZKTree:              zkTree,
		ZKRoots:             zkRoots,
		TrustedPoHAttestors: []crypto.DilithiumPublicKey{attestorKS.PublicKey()},
		EligibilityZK:       eligSys,
		EligibilityTree:     eligTree,
		EligibilityRoots:    zk.NewRootHistory(initialEligRoot),
		MintZK:              mintSys,
		StakeZK:             stakeSys,
		UnstakeZK:           unstakeSys,
		StakeTree:           stakeTree,
		StakeRoots:          zk.NewRootHistory(initialStakeRoot),
	}
	pipeline := tx.NewPipeline(deps)

	v1pk, v1sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen validator key: %v", err)
	}
	v1id := types.NFTID(types.SumHash(v1pk))

	b := &testBackend{
		store: store, chn: chn, deps: deps, pipeline: pipeline, v1id: v1id, v1pk: v1pk, v1sk: v1sk, logf: t.Logf,
		attestorKeystorePath: attestorPath, attestorKeystorePassphrase: "attestor-passphrase",
		eligibilityZKParamsPath: eligParamsPath,
		mintZKParamsPath:        mintParamsPath,
		stakeZKParamsPath:       stakeParamsPath,
		unstakeZKParamsPath:     unstakeParamsPath,
	}

	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("backend host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	shadownet.NewNode(h, nil, b.handle)

	addrs := shadownet.FullAddr(h)
	if len(addrs) == 0 {
		t.Fatalf("backend host has no listen address")
	}
	b.addr = addrs[0]

	srv := query.NewServer(store, chn, tx.NewMempool(), query.Config{ListenAddr: "127.0.0.1:0", GenesisMs: genesisMs, Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start query server: %v", err)
	}
	t.Cleanup(cancel)
	b.queryURL = "http://" + srv.Addr()

	return b
}

func (b *testBackend) handle(_ peer.ID, env shadownet.Envelope) {
	if env.Type != shadownet.MsgTxOffer {
		return
	}
	var payload shadownet.TxOfferPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	txn := payload.Tx

	results := b.pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
	if results[0].Error != nil {
		if b.logf != nil {
			b.logf("backend: pipeline rejected tx %s: %v", txn.TxID, results[0].Error)
		}
		return
	}

	committee := []types.NFTID{b.v1id}
	lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		if id == b.v1id {
			return b.v1pk, true
		}
		return nil, false
	}
	blk := b.chn.NextBlock(0, []types.ShieldedTx{txn}, types.Hash{1}, types.Hash{2}, types.Hash{}, b.v1id, time.Now().UnixMilli())
	candidate := types.HashBlock(blk)
	sig, err := crypto.DilithiumSign(b.v1sk, candidate[:])
	if err != nil {
		return
	}
	blk.Votes = []types.Vote{{Validator: b.v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig)}}
	if err := b.chn.Append(blk, committee, lookup); err != nil && b.logf != nil {
		b.logf("backend: append failed for tx %s: %v", txn.TxID, err)
	}
}

// mustExtractFlagValue parses one "  -flagName value" line out of a real
// CLI command's captured stdout — real output format, not a mock.
func mustExtractFlagValue(t *testing.T, output, flagName string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, flagName+" ") {
			return strings.TrimSpace(strings.TrimPrefix(line, flagName))
		}
	}
	t.Fatalf("expected output to contain %s, got:\n%s", flagName, output)
	return ""
}

// mintNFTViaCLI drives the real, two-role CLI flow end to end: backend's
// real trusted attestor keystore signs a real attestation via 'wallet
// poh-attest', and the requester submits it via 'wallet nft-mint' —
// proving both new commands work together against a real running
// backend, not just that each independently doesn't error.
func mintNFTViaCLI(t *testing.T, backend *testBackend, requesterPK crypto.DilithiumPublicKey, requesterPath, requesterPassphrase string) {
	t.Helper()
	owner := types.AddressFromPubkey(requesterPK)
	const nonce = 1

	withStdin(t, backend.attestorKeystorePassphrase)
	out, err := captureStdout(t, func() error {
		return runPoHAttest([]string{
			"-keystore", backend.attestorKeystorePath, "-passphrase-stdin",
			"-owner", hex.EncodeToString(owner[:]), "-nonce", fmt.Sprintf("%d", nonce),
		})
	})
	if err != nil {
		t.Fatalf("runPoHAttest: %v", err)
	}

	withStdin(t, requesterPassphrase)
	err = runNFTMint([]string{
		"-keystore", requesterPath, "-passphrase-stdin",
		"-nonce", fmt.Sprintf("%d", nonce),
		"-attestation-issued-at-ms", mustExtractFlagValue(t, out, "-attestation-issued-at-ms"),
		"-attestor-pubkey", mustExtractFlagValue(t, out, "-attestor-pubkey"),
		"-attestation-sig", mustExtractFlagValue(t, out, "-attestation-sig"),
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runNFTMint: %v", err)
	}
}

func TestCLIVoteAndVoteRevealEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x01, nil, nil, nil)
	path, ks := newTestKeystore(t, "vote-passphrase")
	mintNFTViaCLI(t, backend, ks.PublicKey(), path, "vote-passphrase")

	withStdin(t, "vote-passphrase")
	err := runVote([]string{
		"-keystore", path, "-passphrase-stdin",
		"-proposal", "cli-prop-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVote: %v", err)
	}

	withStdin(t, "vote-passphrase")
	err = runVoteReveal([]string{
		"-keystore", path, "-passphrase-stdin",
		"-proposal", "cli-prop-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	if backend.chn.HeadHeight() != 3 {
		t.Fatalf("expected 3 real committed blocks (nft-mint + vote + reveal), got height %d", backend.chn.HeadHeight())
	}
}

func TestCLIMintEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x02, nil, nil, nil)
	path, _ := newTestKeystore(t, "mint-passphrase")

	withStdin(t, "mint-passphrase")
	err := runMint([]string{
		"-keystore", path, "-passphrase-stdin",
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runMint: %v", err)
	}
	if backend.chn.HeadHeight() != 1 {
		t.Fatalf("expected 1 real committed block, got height %d", backend.chn.HeadHeight())
	}
}

func TestCLINFTTraitEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x03, nil, nil, nil)
	target := types.ValidatorNFT{ID: types.NFTID{0x55}, Owner: types.Address{0x01}}
	if err := backend.store.PutNFT(target); err != nil {
		t.Fatalf("put target nft: %v", err)
	}

	path, _ := newTestKeystore(t, "trait-passphrase")
	withStdin(t, "trait-passphrase")
	err := runNFTTrait([]string{
		"-keystore", path, "-passphrase-stdin",
		"-target", target.ID.String(), "-key", "dept", "-delta", "5",
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runNFTTrait: %v", err)
	}
	if backend.chn.HeadHeight() != 1 {
		t.Fatalf("expected 1 real committed block, got height %d", backend.chn.HeadHeight())
	}
}

// TestCLIVoteRejectedWithoutRealNFT is the CLI-level proof of the same
// real Sybil-voting fix TestPipelineVoteRejectsUnmintedWallet covers at
// the pipeline layer: a real 'wallet vote' submission from a keystore
// that never ran 'wallet nft-mint' must be rejected by a real running
// backend, not silently accepted.
func TestCLIVoteRejectedWithoutRealNFT(t *testing.T) {
	backend := newTestBackend(t, 0x05, nil, nil, nil)
	path, _ := newTestKeystore(t, "no-nft-passphrase")

	withStdin(t, "no-nft-passphrase")
	err := runVote([]string{
		"-keystore", path, "-passphrase-stdin",
		"-proposal", "cli-sybil-proposal", "-approve",
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "3s",
	})
	if err == nil {
		t.Fatalf("expected a vote from a wallet with no minted NFT to be rejected")
	}
	if backend.chn.HeadHeight() != 0 {
		t.Fatalf("expected no block to have been committed, got height %d", backend.chn.HeadHeight())
	}
}

func TestCLISubmitWithoutBootstrapFails(t *testing.T) {
	path, _ := newTestKeystore(t, "no-bootstrap")
	withStdin(t, "no-bootstrap")
	err := runMint([]string{"-keystore", path, "-passphrase-stdin"})
	if err == nil {
		t.Fatalf("expected an error when neither -bootstrap nor -bootstrap-file is set")
	}
}

// TestCLIProposeMintEndToEnd proves the real spec-17.4 epoch-mint path
// end to end through the actual CLI: a real minted NFT gives a voter
// real anonymous eligibility, 'wallet propose-mint' builds and submits a
// real Groth16-proven mint claim bound to a fresh output note, a matching
// 'wallet vote-reveal' opens the sealed ballot, and — since this build's
// CLI deliberately exposes no epoch-boundary/tally command of its own
// (spec 17.4's tally is a validator-side, not a wallet-side, operation) —
// the test drives the same real backend.pipeline.TallyDueProposals the
// actual validator runs at epoch end, then proves the real note landed in
// the same canonical tree Transfer's own outputs live in.
func TestCLIProposeMintEndToEnd(t *testing.T) {
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	zkRoots := zk.NewRootHistory(initialRoot)
	backend := newTestBackend(t, 0x06, nil, zkTree, zkRoots)

	path, ks := newTestKeystore(t, "mint-voter-passphrase")
	mintNFTViaCLI(t, backend, ks.PublicKey(), path, "mint-voter-passphrase")

	const amount = 2000

	withStdin(t, "mint-voter-passphrase")
	out, err := captureStdout(t, func() error {
		return runProposeMint([]string{
			"-keystore", path, "-passphrase-stdin",
			"-proposal", "cli-mint-1", "-approve", "-amount", fmt.Sprintf("%d", amount),
			"-eligibility-zk-params", backend.eligibilityZKParamsPath,
			"-mint-zk-params", backend.mintZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runProposeMint: %v", err)
	}
	if !strings.Contains(out, "real mint proposal built and proved") {
		t.Fatalf("expected propose-mint output to describe the real note opening, got:\n%s", out)
	}
	wantCommitHex := mustExtractFlagValue(t, out, "-commitment")

	withStdin(t, "mint-voter-passphrase")
	err = runVoteReveal([]string{
		"-keystore", path, "-passphrase-stdin",
		"-proposal", "cli-mint-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	if backend.chn.HeadHeight() != 3 {
		t.Fatalf("expected 3 real committed blocks (nft-mint + propose-mint + reveal), got height %d", backend.chn.HeadHeight())
	}

	remainingBefore := zkTree.Remaining()
	tallied, err := backend.pipeline.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].MintApplied {
		t.Fatalf("expected the real proposal to pass and the real mint to be applied, got %+v", tallied)
	}
	if got := zkTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected the real note the CLI proved to be inserted into the canonical tree, remaining went from %d to %d", remainingBefore, got)
	}
	if got := hex.EncodeToString(tallied[0].MintOutCommit[:]); got != wantCommitHex {
		t.Fatalf("expected the tallied MintOutCommit %s to match the CLI's own reported commitment %s", got, wantCommitHex)
	}

	out, err = captureStdout(t, func() error {
		return runProposal([]string{"-query", backend.queryURL, "-id", "cli-mint-1"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "mint applied: true") {
		t.Fatalf("expected 'wallet proposal' to report the real mint as applied, got:\n%s", out)
	}
}

// TestCLIProposeSlashEndToEnd proves the real spec-10.3 slash path end
// to end through the actual CLI: a real minted NFT gives a voter real
// anonymous eligibility, 'wallet propose-slash' builds and submits a
// real slash request against a second, separately-minted target NFT, a
// matching 'wallet vote-reveal' opens the sealed ballot, and — since
// this build's CLI deliberately exposes no epoch-boundary/tally command
// of its own (see TestCLIProposeMintEndToEnd's identical doc) — the test
// drives the same real TallyDueProposals a live validator runs at epoch
// end, then proves the real target NFT record lands genuinely Slashed.
func TestCLIProposeSlashEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x08, nil, nil, nil)

	voterPath, voterKS := newTestKeystore(t, "slash-voter-passphrase")
	mintNFTViaCLI(t, backend, voterKS.PublicKey(), voterPath, "slash-voter-passphrase")

	targetPath, targetKS := newTestKeystore(t, "slash-target-passphrase")
	mintNFTViaCLI(t, backend, targetKS.PublicKey(), targetPath, "slash-target-passphrase")
	targetOwner := types.AddressFromPubkey(targetKS.PublicKey())
	targetNFT, found, err := backend.store.GetNFTByOwner(targetOwner)
	if err != nil || !found {
		t.Fatalf("expected the target's real minted NFT to be found: found=%v err=%v", found, err)
	}

	withStdin(t, "slash-voter-passphrase")
	out, err := captureStdout(t, func() error {
		return runProposeSlash([]string{
			"-keystore", voterPath, "-passphrase-stdin",
			"-proposal", "cli-slash-1", "-approve", "-target", targetNFT.ID.String(),
			"-eligibility-zk-params", backend.eligibilityZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runProposeSlash: %v", err)
	}
	if !strings.Contains(out, "real slash proposal built") {
		t.Fatalf("expected propose-slash output to describe the real proposal, got:\n%s", out)
	}

	withStdin(t, "slash-voter-passphrase")
	err = runVoteReveal([]string{
		"-keystore", voterPath, "-passphrase-stdin",
		"-proposal", "cli-slash-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	tallied, err := backend.pipeline.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].SlashApplied {
		t.Fatalf("expected the real proposal to pass and the real slash to be applied, got %+v", tallied)
	}

	got, found, err := backend.store.GetNFT(targetNFT.ID)
	if err != nil || !found {
		t.Fatalf("expected the target's NFT record to still exist (frozen, not burned): found=%v err=%v", found, err)
	}
	if !got.Slashed {
		t.Fatalf("expected the real target NFT to be marked Slashed")
	}

	out, err = captureStdout(t, func() error {
		return runProposal([]string{"-query", backend.queryURL, "-id", "cli-slash-1"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "slash applied: true") {
		t.Fatalf("expected 'wallet proposal' to report the real slash as applied, got:\n%s", out)
	}
}

// TestCLIProposeUnlockTransferAndNFTTransferEndToEnd proves the real
// spec-10.1 transfer-unlock-then-transfer path end to end through the
// actual CLI: a real minted NFT gives a voter real anonymous
// eligibility, 'wallet propose-unlock-transfer' builds and submits a
// real unlock request against a second, separately-minted target NFT, a
// matching 'wallet vote-reveal' opens the sealed ballot, the test drives
// the same real TallyDueProposals a live validator runs at epoch end
// (see TestCLIProposeMintEndToEnd's identical doc for why), and only
// THEN does 'wallet nft-transfer', signed by the target's own real
// keystore identity, actually move the NFT to a new owner.
func TestCLIProposeUnlockTransferAndNFTTransferEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x09, nil, nil, nil)

	voterPath, voterKS := newTestKeystore(t, "unlock-voter-passphrase")
	mintNFTViaCLI(t, backend, voterKS.PublicKey(), voterPath, "unlock-voter-passphrase")

	targetPath, targetKS := newTestKeystore(t, "unlock-target-passphrase")
	mintNFTViaCLI(t, backend, targetKS.PublicKey(), targetPath, "unlock-target-passphrase")
	targetOwner := types.AddressFromPubkey(targetKS.PublicKey())
	targetNFT, found, err := backend.store.GetNFTByOwner(targetOwner)
	if err != nil || !found {
		t.Fatalf("expected the target's real minted NFT to be found: found=%v err=%v", found, err)
	}

	withStdin(t, "unlock-voter-passphrase")
	out, err := captureStdout(t, func() error {
		return runProposeUnlockTransfer([]string{
			"-keystore", voterPath, "-passphrase-stdin",
			"-proposal", "cli-unlock-1", "-approve", "-target", targetNFT.ID.String(),
			"-eligibility-zk-params", backend.eligibilityZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runProposeUnlockTransfer: %v", err)
	}
	if !strings.Contains(out, "real transfer-unlock proposal built") {
		t.Fatalf("expected propose-unlock-transfer output to describe the real proposal, got:\n%s", out)
	}

	withStdin(t, "unlock-voter-passphrase")
	err = runVoteReveal([]string{
		"-keystore", voterPath, "-passphrase-stdin",
		"-proposal", "cli-unlock-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	tallied, err := backend.pipeline.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].UnlockTransferApplied {
		t.Fatalf("expected the real proposal to pass and the real unlock to be applied, got %+v", tallied)
	}

	unlocked, found, err := backend.store.GetNFT(targetNFT.ID)
	if err != nil || !found {
		t.Fatalf("get target nft: found=%v err=%v", found, err)
	}
	if unlocked.Traits["transferable"] != "true" {
		t.Fatalf("expected the real transferable trait to be set, got %+v", unlocked.Traits)
	}

	out, err = captureStdout(t, func() error {
		return runProposal([]string{"-query", backend.queryURL, "-id", "cli-unlock-1"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "unlock transfer applied: true") {
		t.Fatalf("expected 'wallet proposal' to report the real unlock as applied, got:\n%s", out)
	}

	newOwner := types.Address{0x42}
	withStdin(t, "unlock-target-passphrase")
	err = runNFTTransfer([]string{
		"-keystore", targetPath, "-passphrase-stdin",
		"-target", targetNFT.ID.String(), "-new-owner", newOwner.String(),
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runNFTTransfer: %v", err)
	}

	moved, found, err := backend.store.GetNFT(targetNFT.ID)
	if err != nil || !found {
		t.Fatalf("get transferred nft: found=%v err=%v", found, err)
	}
	if moved.Owner != newOwner {
		t.Fatalf("expected Owner %s, got %s", newOwner, moved.Owner)
	}
	if _, found, err := backend.store.GetNFTByOwner(targetOwner); err != nil {
		t.Fatalf("get by old owner: %v", err)
	} else if found {
		t.Fatalf("expected the old owner's index entry to be removed after a real CLI transfer")
	}
}

// TestCLIProposeAuthorizeAssetEndToEnd proves the real spec-11/19.3
// Bank-asset-authorization path end to end through the actual CLI: a
// real minted NFT gives a voter real anonymous eligibility, 'wallet
// propose-authorize-asset' builds and submits a real request to
// authorize BTC for Bank use, a matching 'wallet vote-reveal' opens the
// sealed ballot, the test drives the same real TallyDueProposals a live
// validator runs at epoch end (see TestCLIProposeMintEndToEnd's
// identical doc for why), and the real store ends up with BTC
// authorized — proving the CLI's own wiring end to end; the resulting
// Bank-gate enforcement itself (a BankDeposit naming BTC rejected before
// this and accepted after) is already proven directly against the real
// pipeline by pkg/tx's and pkg/txbuilder's own tests, so this test
// doesn't repeat it through 'wallet bank-deposit', which would need a
// live external price oracle this offline test suite has none of.
func TestCLIProposeAuthorizeAssetEndToEnd(t *testing.T) {
	backend := newTestBackend(t, 0x0a, nil, nil, nil)

	voterPath, voterKS := newTestKeystore(t, "authorize-asset-voter-passphrase")
	mintNFTViaCLI(t, backend, voterKS.PublicKey(), voterPath, "authorize-asset-voter-passphrase")

	withStdin(t, "authorize-asset-voter-passphrase")
	out, err := captureStdout(t, func() error {
		return runProposeAuthorizeAsset([]string{
			"-keystore", voterPath, "-passphrase-stdin",
			"-proposal", "cli-authorize-asset-1", "-approve", "-asset", "BTC",
			"-eligibility-zk-params", backend.eligibilityZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runProposeAuthorizeAsset: %v", err)
	}
	if !strings.Contains(out, "real asset-authorization proposal built") {
		t.Fatalf("expected propose-authorize-asset output to describe the real proposal, got:\n%s", out)
	}

	withStdin(t, "authorize-asset-voter-passphrase")
	err = runVoteReveal([]string{
		"-keystore", voterPath, "-passphrase-stdin",
		"-proposal", "cli-authorize-asset-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	tallied, err := backend.pipeline.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].ContainerAssetApplied {
		t.Fatalf("expected the real proposal to pass and the real authorization to be applied, got %+v", tallied)
	}

	authorized, err := backend.store.IsAssetAuthorized(types.AssetID("BTC"))
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	if !authorized {
		t.Fatalf("expected BTC to be authorized in the real store after a passed proposal")
	}

	out, err = captureStdout(t, func() error {
		return runProposal([]string{"-query", backend.queryURL, "-id", "cli-authorize-asset-1"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "container asset applied: true") {
		t.Fatalf("expected 'wallet proposal' to report the real authorization as applied, got:\n%s", out)
	}
}

// TestCLIProposeMintStakedAndUnstakeEndToEnd proves the real spec-17.4
// staked-yield path end to end through the actual CLI: 'wallet
// propose-mint -staked' builds and submits a real, Groth16-proven staked
// mint claim, a matching 'wallet vote-reveal' opens the sealed ballot,
// the test directly drives the same real TallyDueProposals a live
// validator runs at epoch end (this build's CLI has no epoch-boundary
// trigger of its own — see TestCLIProposeMintEndToEnd's identical
// doc), and 'wallet unstake' — using only the position opening
// propose-mint -staked printed, syncing the real stake tree from the
// live network itself via pkg/stakewallet — redeems it for a real
// spendable note that lands in the same canonical tree Transfer's own
// outputs live in.
func TestCLIProposeMintStakedAndUnstakeEndToEnd(t *testing.T) {
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	zkRoots := zk.NewRootHistory(initialRoot)
	backend := newTestBackend(t, 0x07, nil, zkTree, zkRoots)

	path, ks := newTestKeystore(t, "staked-voter-passphrase")
	mintNFTViaCLI(t, backend, ks.PublicKey(), path, "staked-voter-passphrase")

	const amount = 5000

	withStdin(t, "staked-voter-passphrase")
	out, err := captureStdout(t, func() error {
		return runProposeMint([]string{
			"-keystore", path, "-passphrase-stdin", "-staked",
			"-proposal", "cli-staked-1", "-approve", "-amount", fmt.Sprintf("%d", amount),
			"-eligibility-zk-params", backend.eligibilityZKParamsPath,
			"-stake-zk-params", backend.stakeZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runProposeMint -staked: %v", err)
	}
	if !strings.Contains(out, "real staked mint proposal built and proved") {
		t.Fatalf("expected propose-mint -staked output to describe the real position opening, got:\n%s", out)
	}
	principalStr := mustExtractFlagValue(t, out, "-principal")
	startEpochStr := mustExtractFlagValue(t, out, "-start-epoch")
	ownerSKHex := mustExtractFlagValue(t, out, "-owner-sk")
	rhoHex := mustExtractFlagValue(t, out, "-rho")
	if principalStr != fmt.Sprintf("%d", amount) {
		t.Fatalf("expected printed principal %d, got %s", amount, principalStr)
	}

	withStdin(t, "staked-voter-passphrase")
	err = runVoteReveal([]string{
		"-keystore", path, "-passphrase-stdin",
		"-proposal", "cli-staked-1", "-approve",
		"-eligibility-zk-params", backend.eligibilityZKParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runVoteReveal: %v", err)
	}

	remainingBefore := backend.deps.StakeTree.Remaining()
	tallied, err := backend.pipeline.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].MintApplied || !tallied[0].MintStaked {
		t.Fatalf("expected the real staked proposal to pass and the real position to be applied, got %+v", tallied)
	}
	if got := backend.deps.StakeTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected the real position the CLI proved to land in the stake tree, remaining went from %d to %d", remainingBefore, got)
	}

	out, err = captureStdout(t, func() error {
		return runProposal([]string{"-query", backend.queryURL, "-id", "cli-staked-1"})
	})
	if err != nil {
		t.Fatalf("runProposal: %v", err)
	}
	if !strings.Contains(out, "mint staked: true") || !strings.Contains(out, "mint applied: true") {
		t.Fatalf("expected 'wallet proposal' to report the real staked mint as applied, got:\n%s", out)
	}

	remainingNotesBefore := zkTree.Remaining()
	withStdin(t, "staked-voter-passphrase")
	out, err = captureStdout(t, func() error {
		return runUnstake([]string{
			"-keystore", path, "-passphrase-stdin",
			"-principal", principalStr, "-start-epoch", startEpochStr,
			"-owner-sk", ownerSKHex, "-rho", rhoHex,
			"-unstake-zk-params", backend.unstakeZKParamsPath,
			"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
		})
	})
	if err != nil {
		t.Fatalf("runUnstake: %v", err)
	}
	if !strings.Contains(out, "real unstake built and proved") {
		t.Fatalf("expected unstake output to describe the real redemption, got:\n%s", out)
	}
	if got := zkTree.Remaining(); got != remainingNotesBefore-1 {
		t.Fatalf("expected the real redeemed note to land in the canonical note tree, remaining went from %d to %d", remainingNotesBefore, got)
	}
}

// --- real shielded balance/transfer, driven by the actual CLI commands ---

// TestCLIBalanceAndTransferEndToEnd proves the CLI's own "balance" and
// "transfer" commands work against real chain data, entirely through
// real Sync — no capability this CLI doesn't itself expose. This build
// has no real, callable mint mechanism (see pkg/shieldedwallet's own
// doc), so a "seed" identity is funded by a one-time genesis event
// delivered as an actual committed block (built directly against
// backend.chn here, the way a real chain's genesis coinbase would be,
// not through this CLI, which deliberately exposes no bootstrap/import
// command — adding one would be a real backdoor around the canonical-
// tree soundness check). Landing it in a real block, rather than using
// pkg/shieldedwallet's own ImportCanonicalNote off-chain bypass, means
// every wallet below — including the receiver, driven entirely through
// this binary's real CLI commands — reconstructs an identically
// structured local tree purely by replaying real blocks, exactly as a
// genuine live user's wallet would. From "wallet balance" onward, every
// step is the actual CLI: discovering a real transfer via Sync, then
// itself building and submitting a further real Groth16-proven transfer
// to a third identity, confirmed the same way a genuine user's would be.
func TestCLIBalanceAndTransferEndToEnd(t *testing.T) {
	zkSys, err := zk.Setup()
	if err != nil {
		t.Fatalf("zk setup: %v", err)
	}
	// The CLI's own "transfer" command never runs its own zk.Setup() (see
	// zk-setup's doc on why: an independent setup could never verify
	// against this test's real backend) — it loads real, shared params
	// from a file instead, exactly like a genuine deployment's shared
	// 'wallet zk-setup' output would be.
	zkParamsPath := filepath.Join(t.TempDir(), "zk-params.bin")
	zkParamsFile, err := os.Create(zkParamsPath)
	if err != nil {
		t.Fatalf("create zk params file: %v", err)
	}
	if _, err := zkSys.WriteTo(zkParamsFile); err != nil {
		t.Fatalf("write zk params: %v", err)
	}
	if err := zkParamsFile.Close(); err != nil {
		t.Fatalf("close zk params file: %v", err)
	}
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	zkRoots := zk.NewRootHistory(initialRoot)
	backend := newTestBackend(t, 0x04, zkSys, zkTree, zkRoots)

	seedPK, seedSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("gen seed dilithium: %v", err)
	}
	seedXSK, err := ecdh.X25519().GenerateKey(cryptorand.Reader)
	if err != nil {
		t.Fatalf("gen seed x25519: %v", err)
	}
	seedWallet, err := shieldedwallet.New(seedPK, seedSK, seedXSK.PublicKey(), seedXSK, shieldedwallet.Config{QueryBase: backend.queryURL})
	if err != nil {
		t.Fatalf("new seed wallet: %v", err)
	}

	// Genesis funding, delivered as a real committed block rather than
	// pkg/shieldedwallet's own ImportCanonicalNote off-chain bypass: this
	// build has no real, callable mint mechanism (see that package's own
	// doc), but a one-time genesis funding event landing in an actual
	// block — the way a real chain's genesis coinbase would — is the one
	// form of bootstrap every wallet's ordinary Sync can independently
	// discover and correctly index, with no special-casing. That matters
	// here specifically because the receiver below is driven entirely
	// through this binary's own CLI commands, which deliberately expose
	// no bootstrap/import capability (adding one would be a real backdoor
	// around the canonical-tree soundness check) — so its local tree must
	// end up structurally identical to the real one purely by replaying
	// real blocks, exactly as a genuine live user's wallet would.
	values := []uint64{60, 40, 20, 15}
	secrets := make([]zk.NoteSecret, len(values))
	for i, v := range values {
		sk, err := zk.NewSpendKey()
		if err != nil {
			t.Fatal(err)
		}
		rho, err := zk.NewRho()
		if err != nil {
			t.Fatal(err)
		}
		secrets[i] = zk.NoteSecret{Value: v, OwnerSK: sk, Rho: rho}
	}
	outCommits := make([]types.Hash, len(secrets))
	receiverPubs := make([]*ecdh.PublicKey, len(secrets))
	for i, s := range secrets {
		outCommits[i] = types.Hash(zk.ToBytes32(s.Commitment()))
		if _, err := zkTree.Insert(s.Commitment()); err != nil {
			t.Fatalf("seed canonical tree: %v", err)
		}
		receiverPubs[i] = seedXSK.PublicKey()
	}
	root, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	zkRoots.Record(root)

	genesisMemo, err := shieldedwallet.EncryptMemos(receiverPubs, secrets)
	if err != nil {
		t.Fatalf("encrypt genesis memos: %v", err)
	}
	genesisTx := types.ShieldedTx{
		Kind:                 types.TxTransfer,
		Commitments:          outCommits,
		Nullifier:            types.SumHash([]byte("test-genesis-funding-event")),
		TransferPublicInputs: &types.TransferPublicInputs{MerkleRoot: types.Hash(zk.ToBytes32(root)), OutCommits: outCommits},
		Memo:                 genesisMemo,
	}
	genesisTx.TxID = types.ComputeTxID(genesisTx.Proof, genesisTx.Commitments, genesisTx.Nullifier)
	committeeOnlyV1 := []types.NFTID{backend.v1id}
	lookupOnlyV1 := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
		if id == backend.v1id {
			return backend.v1pk, true
		}
		return nil, false
	}
	genesisBlk := backend.chn.NextBlock(0, []types.ShieldedTx{genesisTx}, types.Hash{1}, types.Hash{2}, types.Hash{}, backend.v1id, time.Now().UnixMilli())
	genesisCandidate := types.HashBlock(genesisBlk)
	genesisSig, err := crypto.DilithiumSign(backend.v1sk, genesisCandidate[:])
	if err != nil {
		t.Fatalf("sign genesis block: %v", err)
	}
	genesisBlk.Votes = []types.Vote{{Validator: backend.v1id, StateRoot: genesisCandidate, Sig: types.DilithiumSig(genesisSig)}}
	if err := backend.chn.Append(genesisBlk, committeeOnlyV1, lookupOnlyV1); err != nil {
		t.Fatalf("append genesis funding block: %v", err)
	}

	ctx := context.Background()
	if err := seedWallet.Sync(ctx); err != nil {
		t.Fatalf("seed wallet sync: %v", err)
	}
	if seedWallet.Balance() != 60+40+20+15 {
		t.Fatalf("expected seed wallet to discover its real genesis funding via Sync, got balance %d", seedWallet.Balance())
	}

	receiverPath, receiverKS := newTestKeystore(t, "receiver-passphrase")
	receiverID, err := receiverKS.UnlockShielded("receiver-passphrase")
	if err != nil {
		t.Fatalf("unlock receiver: %v", err)
	}

	// submitDirect drives the real pipeline and a real single-validator
	// BFT commit directly (the same real commit path testBackend.handle
	// uses for genuine wire traffic) — used here for the seed wallet's
	// own real, Groth16-proven transfers funding the receiver, so this
	// test isn't solely dependent on the libp2p submission path already
	// proven by the other CLI end-to-end tests in this file.
	submitDirect := func(txn types.ShieldedTx) {
		t.Helper()
		results := backend.pipeline.ProcessBatch([]tx.Entry{{Tx: txn}})
		if results[0].Error != nil {
			t.Fatalf("pipeline rejected seed transfer: %v", results[0].Error)
		}
		committee := []types.NFTID{backend.v1id}
		lookup := func(id types.NFTID) (crypto.DilithiumPublicKey, bool) {
			if id == backend.v1id {
				return backend.v1pk, true
			}
			return nil, false
		}
		blk := backend.chn.NextBlock(0, []types.ShieldedTx{txn}, types.Hash{1}, types.Hash{2}, types.Hash{}, backend.v1id, time.Now().UnixMilli())
		candidate := types.HashBlock(blk)
		sig, err := crypto.DilithiumSign(backend.v1sk, candidate[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		blk.Votes = []types.Vote{{Validator: backend.v1id, StateRoot: candidate, Sig: types.DilithiumSig(sig)}}
		if err := backend.chn.Append(blk, committee, lookup); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Fund the receiver with 2 real, chain-committed transfers so it has
	// enough known notes (this build's fixed 2-input circuit) to drive a
	// further transfer itself.
	txn1, err := seedWallet.BuildTransfer(zkSys, receiverID.ShieldedPub, 70, 5)
	if err != nil {
		t.Fatalf("seed build transfer 1: %v", err)
	}
	submitDirect(txn1)
	txn2, err := seedWallet.BuildTransfer(zkSys, receiverID.ShieldedPub, 10, 1)
	if err != nil {
		t.Fatalf("seed build transfer 2: %v", err)
	}
	submitDirect(txn2)

	// From here on, every step is the actual CLI.
	withStdin(t, "receiver-passphrase")
	out, err := captureStdout(t, func() error {
		return runBalance([]string{"-keystore", receiverPath, "-passphrase-stdin", "-query", backend.queryURL})
	})
	if err != nil {
		t.Fatalf("runBalance: %v", err)
	}
	if !strings.Contains(out, "balance: 80") {
		t.Fatalf("expected CLI balance 80 (70+10), got:\n%s", out)
	}

	thirdPath, thirdKS := newTestKeystore(t, "third-passphrase")

	withStdin(t, "receiver-passphrase")
	err = runTransfer([]string{
		"-keystore", receiverPath, "-passphrase-stdin",
		"-to", hex.EncodeToString(thirdKS.ShieldedPublicKey().Bytes()),
		"-amount", "10", "-fee", "1", "-zk-params", zkParamsPath,
		"-bootstrap", backend.addr, "-query", backend.queryURL, "-confirm-timeout", "10s",
	})
	if err != nil {
		t.Fatalf("runTransfer: %v", err)
	}

	withStdin(t, "third-passphrase")
	out, err = captureStdout(t, func() error {
		return runBalance([]string{"-keystore", thirdPath, "-passphrase-stdin", "-query", backend.queryURL})
	})
	if err != nil {
		t.Fatalf("runBalance for third identity: %v", err)
	}
	if !strings.Contains(out, "balance: 10") {
		t.Fatalf("expected the third identity's CLI-reported balance to be 10 after a real CLI-submitted transfer, got:\n%s", out)
	}
}
