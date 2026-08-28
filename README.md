# ShadowForge L1

A privacy-by-default, NFT-authorized, epoch-paced Layer 1 blockchain — built
from the [ShadowForge L1 Master Specification](docs/SPEC_SOURCE.md) v3.0.
No fork of Solana, Ethereum, or Cosmos: a from-scratch consensus layer, a
real zero-knowledge shielded-transfer circuit, a domain-specific language
(ShadowRust) compiled to Go, and a volatility firewall (the Bank) between
external crypto assets and the native token, SFG.

This repository is the L1 core: the node, its consensus, its ZK circuit,
its transaction pipeline, its P2P layer, and the ShadowRust toolchain that
compiles to it. See [Scope](#scope) for what's deliberately not in this
repository (the Flutter wallet, the Creation App, and the ShadeLang visual
editor are separate, later-phase deliverables per the spec's own phasing).

## Quickstart

```sh
go build ./...          # compiles every package and command
go test ./...            # full test suite (~15s; pkg/zk's real Groth16
                          # proving is the slow part)
go run ./cmd/hello        # Phase 0 toolchain sanity check
```

Run a single node:

```sh
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/4001
```

Run the ShadowRust CLI against the canonical example from spec section
14.5:

```sh
go run ./cmd/shadowc parse  examples/sample_transfer.sr
go run ./cmd/shadowc lint   examples/sample_transfer.sr
go run ./cmd/shadowc interp examples/sample_transfer.sr
go run ./cmd/shadowc gen    examples/sample_transfer.sr
```

Run a four-node network (two civilian validators, one sentinel, one wallet
simulator — the exact topology spec section 6 calls for) as local
processes, without Docker. The three validator-role nodes are wired as a
full mesh (see `deployments/docker/docker-compose.yml`'s comment for why
this reference build needs that, not a star, for every node's committee
view to agree):

```sh
mkdir -p /tmp/shared
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15001 -announce-file /tmp/shared/v1.addr -skip-zk-setup &
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15003 -bootstrap-file /tmp/shared/v1.addr -announce-file /tmp/shared/s1.addr -sentinel -skip-zk-setup &
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15002 -bootstrap-file /tmp/shared/v1.addr,/tmp/shared/s1.addr -announce-file /tmp/shared/v2.addr -skip-zk-setup &
go run ./cmd/walletsim -listen /ip4/127.0.0.1/tcp/15010 -bootstrap-file /tmp/shared/v2.addr -interval 1s
```

Watch any node's log for `chain height=N hash=...` — all three should
converge on the same height and hash as `walletsim`'s transactions get
proposed, voted on, and committed via real cross-node BFT consensus
(`pkg/validator`).

Or with Docker (see [deployments/docker](deployments/docker)):

```sh
docker compose -f deployments/docker/docker-compose.yml up --build
```

> The Docker Compose file and Dockerfile were authored and reviewed but
> could not be built or run in the sandbox this repository was produced in
> (no Docker daemon was available there). The exact topology and bootstrap
> mechanism it uses — including the full-mesh validator wiring real BFT
> consensus requires — were verified working via direct process execution:
> all three validator-role nodes reached identical chain height and head
> hash after real signed transactions, real Dilithium-signed votes, and
> real quorum-gated commits, growing the chain to height 12 in one run.
> The transcript is reproducible with the local-process command above.

## What's real here

Every claim below is backed by a passing test in the corresponding
package — this is not a stub with comments describing intended behavior.

- **A real zero-knowledge circuit.** `pkg/zk` is a Groth16 circuit (gnark,
  BN254) that proves, without revealing sender, receiver, or amount, all
  five properties spec section 8.1 requires: Merkle membership of spent
  notes, knowledge of the note opening, correct nullifier derivation,
  value conservation, and well-formed new commitments. `pkg/zk/zk_test.go`
  compiles the circuit, runs a Groth16 setup, proves a real two-input/
  two-output transfer, verifies it, and confirms a tampered witness or
  public input is rejected.
- **A real ShadowRust compiler.** `grammar/ShadowRust.g4` is the pinned
  core grammar from spec 14.2, extended via ANTLR import
  (`grammar/ShadowRustExt.g4`) rather than edited in place. The generated
  parser (`parser/`), typed AST builder (`ast/`), sandbox interpreter
  (`interp/`), semantic analyzer (`sema/`), and Go code generator
  (`codegen/`) form a working pipeline: `codegen`'s test suite actually
  `go build`s its own generated output.
- **A real P2P layer.** `pkg/net` builds actual libp2p hosts, completes a
  real Noise XX handshake between them, and exchanges an encrypted,
  decoded application message — proven in `pkg/net/net_test.go` with two
  live hosts, not mocks.
- **Real cross-node BFT consensus.** `pkg/validator` runs a genuine
  propose/vote/commit state machine across independent nodes: a
  deterministically-assigned proposer (`pkg/consensus.AssignCommittee`, a
  pure function every honest node computes identically), real
  Dilithium-signed `StageVote`s exchanged over libp2p, and a quorum-gated,
  independently-reverifying `pkg/chain.Append` — a node never counts a
  vote it hasn't itself checked against a real public key and real
  committee membership. `TestFourNodesConvergeOnSameChain`
  (`pkg/validator/integration_test.go`) proves four fully independent
  `validator.Node`s, wired only by real TCP/Noise sockets, converge on
  byte-identical chain height and head hash; the same real `cmd/node`
  binary was separately verified as three full-mesh OS processes plus a
  wallet simulator, growing a real chain to height 12 (see the Quickstart
  section above).
- **A real five-stage pipeline with atomicity and real signatures.**
  `pkg/tx` runs every transaction through Sender Leave → TX Offer →
  Receiver Check → Send Exec → Place Final (spec 5.3), verifying the
  actual ZK proof and a real Dilithium signature (over a TxID that must
  itself match `Hash(proof||commitments||nullifier)`, spec 4.1) at
  Stage 1/2, and releasing its nullifier reservation if any later stage
  fails — tested with a real proof, a tampered signature, a tampered
  TxID, and a forced later-stage failure with a check that the nullifier
  was released, not stuck.
- **Exact formulas.** `pkg/consensus` (epoch duration, revolver
  insert), `pkg/bank` (deposit/withdraw ATR math), and `pkg/vault`
  (fee splits, bonus multiplier) implement spec section 19's formulas
  using exact rational arithmetic (`pkg/decimal`, `math/big.Rat`) rather
  than floats, with tests against the spec's own worked examples.
- **Real fee collection, spike defense, and governance tallying — not
  just correct logic sitting unused.** A committed Transfer's real,
  ZK-proven `FeeAmount` is routed into `vault.CollectFee`'s 20/10/10/60
  split at Stage 5. Every signature-verified transaction is recorded
  against a real `pkg/silent.RateMonitor` (keyed off the address the
  signature check just proved genuine, never an unverified claimed
  pubkey), which auto-establishes a baseline per wallet and places a
  real, queryable hold the moment a burst is flagged. Vote-kind
  transactions implement a real sealed-ballot commit-reveal
  (`types.ComputeVoteCommitment`, a `TxVoteReveal` kind checked
  cryptographically against the earlier commitment) feeding a real
  epoch-boundary tally (`Pipeline.TallyDueProposals`) that's
  deterministic across every honest node and wired into both
  `pkg/validator`'s propose and announce-replay paths — proven with a
  test that waits for a genuine wall-clock epoch boundary to arrive
  before asserting the tally ran.
- **Real, enforced, anonymous voter eligibility — closing a genuine
  Sybil-voting gap live testing found, then closing the identity leak
  the first fix itself introduced.** Casting a ballot originally needed
  nothing but a freshly generated keypair. The first fix
  (`requireEligibleVoter`) closed that by requiring the signer to hold a
  real, minted `ValidatorNFT`, looked up via a real secondary store index
  — sound, but it permanently tied every ballot to a public, long-lived
  wallet address (the tx's own signature revealed who cast it). This
  build now replaces that with a real anonymous zero-knowledge membership
  proof: `pkg/zk.EligibilityCircuit` (a real Groth16 circuit, structurally
  the same Merkle-membership-plus-nullifier shape as the shielded
  Transfer circuit) proves "the caster holds a real, minted NFT" without
  revealing which one, and `requireEligibleVoterZK`
  (`pkg/tx/pipeline.go`) verifies it — an observer of a `TxVote`/
  `TxVoteReveal` learns only that some real NFT voted, never which one,
  because the transaction is signed with a fresh, throwaway key
  unrelated to the identity that minted the NFT (`types.
  VoteEligibilityProof`'s own doc). The eligibility-tree leaf a wallet
  proves membership of (`NFTMintPublicInputs.VoterCommitment`) is
  registered by the same real `Kind NFTMint` path spec 10.1 describes —
  a real, signed proof-of-humanity attestation
  (`pkg/nft.PoHAttestation`/`SignPoHAttestation`, checked against a
  node's `-poh-attestor-keys`) still gates who can mint one; the actual
  CAPTCHA/human-verification challenge stays App-layer/out of scope per
  spec. `pkg/govwallet.Wallet` is the real, network-syncing client that
  replays every committed `NFTMint`'s `VoterCommitment` to build a real
  proof (mirroring `pkg/shieldedwallet`'s own sync-and-prove design for
  shielded transfers); `cmd/wallet poh-attest`/`nft-mint`/`vote`/
  `vote-reveal`/`eligibility-zk-setup` are the real CLI for the whole
  flow. A real, disclosed limitation of the anonymous design: because a
  valid proof never reveals which leaf it opens, a node cannot re-check
  whether that specific NFT has since been slashed — see
  `requireEligibleVoterZK`'s own doc. `cmd/walletsim`'s throwaway-per-
  session identities still can't vote (no real NFT ever backs them), a
  disclosed trade-off unrelated to this fix — see that command's own doc.
- **Real spec-17.4 epoch-mint execution — closing the last "passes tally
  but does nothing" gap.** A mint proposal used to reach exactly the same
  dead end every proposal type once did: it could be built, voted on, and
  pass tally, with no code ever crediting SFG to anyone. This build wires
  the proposer-direct path (spec 17.4's other option, a staked 2% yield
  path, is left out on the same disclosed boundary as before — it needs a
  staking subsystem this L1-core build doesn't implement anywhere).
  `pkg/zk.MintCircuit` (a real Groth16 circuit, deliberately the simplest
  one in this codebase — it proves only `OutCommit == MiMC(Amount,
  OwnerPK, Rho)`, no Merkle membership, since minting creates value rather
  than conserving it) binds a requested amount to a fresh output note
  using the exact same commitment formula the Transfer circuit's own
  outputs use, so a minted note is an ordinary spendable `zk.NoteSecret`
  — zero new note types, zero changes to the spend path. Following the
  same "a proposal is whatever the first `TxVote` claims" pattern the
  existing ParamChange handling already established, `types.
  VotePublicInputs` gained optional `MintAmount`/`MintOutCommit`/
  `MintProof` fields rather than a new proposal-submission transaction
  kind; the real proof is checked once, at the moment the first vote
  binds the claim (fail-fast, never deferred to tally time), and the note
  is actually inserted into the canonical tree — with a real 10% Vault
  fee collected (`types.MintFeeAmount`, exact-integer floor division) —
  only in `TallyDueProposals`, gated on the proposal having passed, with
  `state.ProposalRecord.MintApplied` as the idempotency marker mirroring
  `Applied`'s existing role for ParamChange. `pkg/txbuilder.
  Builder.ProposeMint`/`cmd/wallet propose-mint`/`mint-zk-setup` are the
  real CLI, reusing the same two-signer separation `vote`/`vote-reveal`
  already established (a throwaway key signs the envelope; the real
  anonymous eligibility proof is built separately against the identity
  that minted the NFT). The pre-existing `types.TxMint`/`Builder.Mint`/
  `wallet mint` are untouched, vestigial scaffolding that predates this
  pattern — not the real mechanism, and documented as such everywhere
  they appear. A real, disclosed, pre-existing gap this work surfaced
  while checking how a landed note could be verified: `state.
  Store.PutNote` is never called by any production code path for any
  note kind, so the `/v1/note/{commitment}` query endpoint cannot confirm
  a minted (or transferred) note landed — `wallet proposal -id` showing
  `mint applied: true` is the real, working way to confirm a mint
  executed.

## Security

A manual review against spec 8.6's STRIDE threat model found and fixed
three real, exploitable gaps rather than being a formality:

1. **Spoofing/Tampering.** Stage 2 originally only checked that a
   signature byte string was non-empty — never that it actually verified.
   Fixed: every transaction's TxID must match
   `Hash(proof || commitments || nullifier)` (spec 4.1) and its Dilithium
   signature must verify against the claimed signer key before Stage 2
   admits it (`pkg/tx/pipeline.go`, `pkg/types.ComputeTxID`).
2. **Denial of service.** `pkg/net`'s stream handler had no per-message
   size cap and no idle timeout (a peer could stream unbounded data, or
   open a stream and hold a goroutine open forever by sending nothing).
   `pkg/tx.Mempool` had no capacity cap (a Sybil flood across many peer
   identities could exhaust memory even with per-peer rate limiting).
   Both are now bounded, with tests proving the bound actually holds.
3. **A completeness gap the review surfaced**: Kind Vote transactions
   passed all five stages but had no Stage 4 effect — cast ballots were
   silently discarded. Fixed by persisting each ballot's commitment
   against its proposal (`state.ProposalRecord`); tallying still
   correctly happens at epoch end, not per-transaction (spec 17.4).

No secrets are logged; encryption keys use `crypto/rand`, never
`math/rand`; AEAD nonces are fresh per call. See `docs/ARCHITECTURE.md`
for what's still required before a real deployment (a production ZK
trusted-setup ceremony and the spec 18.6 hardening phase in full — this
is a tested reference implementation, not an audited production system).

## Repository layout

```
grammar/            ShadowRust.g4 (pinned core) + ShadowRustExt.g4 (dialects)
parser/              ANTLR-generated Go lexer/parser/visitor
ast/ ir/* sema/ interp/ codegen/   ShadowRust compiler pipeline
cmd/hello/           Phase 0 toolchain sanity binary
cmd/shadowc/         ShadowRust CLI: parse, lint, interp, gen
cmd/node/            L1 validator node entrypoint
cmd/walletsim/       lightweight wallet traffic simulator (spec 6, Phase 2 net)
pkg/types/           canonical spec-4 data model structs
pkg/decimal/         exact rational arithmetic
pkg/crypto/          Dilithium3 (PQC) signatures, AEAD encryption
pkg/zk/              Groth16 shielded-transfer circuit + prover/verifier
pkg/state/           encrypted Badger KV store + Merkle tree
pkg/consensus/       epoch clock, revolver, BFT quorum rule, deterministic committee assignment, sentinels, outage/megabatch
pkg/net/             libp2p + Noise, message protocol, rate limiter
pkg/chain/           block assembly, PrevHash-linked growth, genesis, quorum-gated Append
pkg/validator/       cross-node propose/vote/commit state machine (real network BFT consensus)
pkg/tx/              mempool + five-stage pipeline
pkg/bank/ pkg/oracle/  ATR deposit/withdraw math, oracle quorum, hashed-IP correlation
pkg/vault/           fee treasury and splits
pkg/nft/             soulbound mint/traits/Trust Points/slashing
pkg/governance/       genesis parameters, NFT-weighted voting
pkg/govwallet/         real network-syncing client for anonymous voter-eligibility proofs (Kind Vote/VoteReveal)
pkg/container/        enterprise L1 container subspace
pkg/silent/            Poisson silent-TX padding + wallet spike detection (spec 15.4)
deployments/docker/   Dockerfile + docker-compose.yml (4-node network)
docs/                 architecture notes, spec source, scope decisions
examples/             sample .sr programs
```

(`ir/` is reserved for future IR transforms per spec 14.6's compiler module
layout; the current pipeline compiles the AST directly to Go, which is
sufficient for the statement kinds in scope today.)

## Scope

This build focuses effort on the L1 core — the part of the spec that is
genuinely "a blockchain" and that a test suite can hold to account. Per
the spec's own phasing (18.3–18.5: ShadowRust MVP and L1 core are Phase
1–2; the wallet is explicitly "a later Phase 3 track"; the Creation App and
ShadeLang studio are Phase 3 application-layer work in a separate Flutter/
JS codebase), the following are **not** implemented here:

- The Flutter mobile wallet (spec section 12).
- The Creation App and ShadeLang Blockly/Monaco studio (spec section 13).
- The full enterprise-migration tooling (parser scripts against a live
  legacy server, the Bridge Proxy) — `pkg/container` implements the
  runtime subspace mechanics (validator counts, hybrid split, sync
  triggers, shadow verification) that tooling would drive.

Within the L1 core, one thing is implemented but intentionally scoped
down, documented at the point of decision in code:

- **ZK circuit size.** `pkg/zk.MerkleDepth` is 4 (16 leaves), not a
  production depth. Spec 23's own risk register names "tiny circuits" as
  the explicit Year-1 mitigation for "gnark / circuit bugs" — this
  follows that directive. Raising it is a one-constant change plus a new
  trusted setup.

Outage detection and megabatch recovery (spec 5.6) are wired into live
consensus: `pkg/validator`'s round loop calls `pkg/consensus`'s outage
controller every tick (`evaluateOutage`), and a validator that detects an
outage builds a real dual-track proposal (`buildProposalBatch`'s
`dualTrack` path, `OutageController.BuildMegabatch`) until a clean cycle
reaches real BFT quorum. Still intentionally out of scope, per
`pkg/validator/node.go`'s own doc: multi-block catch-up sync for a node
that falls more than one block behind (`handleBlockAnnounce` only
replay-adopts one block at a time), and `MegabatchPart`'s chunked
wire-format reassembly for a recovery batch too large for a single
`MaxBatchBytes`-bounded proposal.

Cross-process BFT finality (spec 5.7) is fully wired and network-tested:
`pkg/validator` runs a real propose/vote/commit state machine — genuine
Dilithium-signed `StageVote` messages exchanged over libp2p, independently
re-verified signatures and committee membership on every node
(`pkg/chain.Append`), and rollback of any round that doesn't reach quorum.
`pkg/validator`'s package doc and its `TestFourNodesConvergeOnSameChain`
integration test cover this end to end across real, independently-driven
network peers, and `deployments/docker/docker-compose.yml`'s three
validator-role nodes run it as a genuine multi-process network (see that
file's comment for why they're wired as a full mesh, not a star: this
reference build doesn't relay heartbeats or messages beyond directly
connected peers, so every validator needs a direct connection to every
other validator for their committee views to agree).

Every other statement kind, formula, and safeguard named in the spec —
Bank deposit/withdraw math, Vault splits, epoch clock, revolver scatter
insert, sentinel activation, outage/megabatch recovery, NFT lifecycle,
governance tally, container subspace mechanics, Dilithium signatures,
encrypted state — is implemented and tested in the packages above.

## Testing

```sh
go build ./...
go vet $(go list ./... | grep -v '^github.com/shadowforge/shadowforge-l1/parser$')
go test ./... -count=1
```

(`go vet` on `parser/` reports pre-existing "unreachable code" findings
inside ANTLR-generated code, not hand-written code; see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#known-vet-finding-in-generated-code).)

See [docs/TESTING.md](docs/TESTING.md) for how each row of the spec's
section 20 Testing Matrix maps to an actual test.
