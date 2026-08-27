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
processes, without Docker:

```sh
mkdir -p /tmp/shared
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15001 -announce-file /tmp/shared/v1.addr -skip-zk-setup &
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15002 -bootstrap-file /tmp/shared/v1.addr -announce-file /tmp/shared/v2.addr -skip-zk-setup &
go run ./cmd/node -listen /ip4/127.0.0.1/tcp/15003 -bootstrap-file /tmp/shared/v1.addr -sentinel -skip-zk-setup &
go run ./cmd/walletsim -listen /ip4/127.0.0.1/tcp/15010 -bootstrap-file /tmp/shared/v2.addr
```

Or with Docker (see [deployments/docker](deployments/docker)):

```sh
docker compose -f deployments/docker/docker-compose.yml up --build
```

> The Docker Compose file and Dockerfile were authored and reviewed but
> could not be built or run in the sandbox this repository was produced in
> (no Docker daemon was available there). The exact topology and bootstrap
> mechanism it uses were verified working via direct process execution —
> the transcript is reproducible with the local-process command above.

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
- **A real five-stage pipeline with atomicity.** `pkg/tx` runs every
  transaction through Sender Leave → TX Offer → Receiver Check → Send Exec
  → Place Final (spec 5.3), verifying the actual ZK proof at Stage 1 and
  releasing its nullifier reservation if any later stage fails — tested
  with a real proof, a forced Stage 2 failure, and a check that the
  nullifier was released, not stuck.
- **Exact formulas.** `pkg/consensus` (epoch duration, revolver
  insert), `pkg/bank` (deposit/withdraw ATR math), and `pkg/vault`
  (fee splits, bonus multiplier) implement spec section 19's formulas
  using exact rational arithmetic (`pkg/decimal`, `math/big.Rat`) rather
  than floats, with tests against the spec's own worked examples.

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
pkg/consensus/       epoch clock, revolver, BFT, sentinels, outage/megabatch
pkg/net/             libp2p + Noise, message protocol, rate limiter
pkg/tx/              mempool + five-stage pipeline
pkg/bank/ pkg/oracle/  ATR deposit/withdraw math, oracle quorum
pkg/vault/           fee treasury and splits
pkg/nft/             soulbound mint/traits/Trust Points/slashing
pkg/governance/       genesis parameters, NFT-weighted voting
pkg/container/        enterprise L1 container subspace
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

Within the L1 core, two things are implemented but intentionally scoped
down, documented at the point of decision in code:

- **ZK circuit size.** `pkg/zk.MerkleDepth` is 4 (16 leaves), not a
  production depth. Spec 23's own risk register names "tiny circuits" as
  the explicit Year-1 mitigation for "gnark / circuit bugs" — this
  follows that directive. Raising it is a one-constant change plus a new
  trusted setup.
- **Cross-process BFT finality.** `pkg/consensus` fully implements and
  tests the BFT quorum rule (spec 5.7); `cmd/node` does not yet exchange
  `StageVote` messages over the network to gate commit on a live quorum
  across physical machines — each node finalizes its own admitted batch
  locally today. Wiring the already-tested quorum logic into the
  already-working message protocol is the natural next step, not a
  redesign.

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
