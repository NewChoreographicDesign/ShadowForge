# Architecture

This document maps the spec's layer model (section 3.1) onto the packages
in this repository, and records a few implementation decisions that
deviate from a literal reading of the spec, with the reasoning for each.

## Layer map → packages

| Spec layer (3.1)                           | Package(s)                                   |
|---------------------------------------------|-----------------------------------------------|
| Tooling layer (grammar, parser, codegen)     | `grammar/`, `parser/`, `ast/`, `sema/`, `interp/`, `codegen/`, `cmd/shadowc/` |
| Transaction layer (mempool, pipeline, ZKP)   | `pkg/tx/`, `pkg/zk/`                          |
| Consensus layer (epoch, revolver, BFT, sentinel, outage) | `pkg/consensus/`                 |
| State layer (encrypted account map, Merkle)  | `pkg/state/`                                  |
| Network layer (P2P, encryption, rate limits) | `pkg/net/`                                    |
| Application layer (NFT, Bank, Vault, governance) | `pkg/nft/`, `pkg/bank/`, `pkg/oracle/`, `pkg/vault/`, `pkg/governance/`, `pkg/container/` |
| Client / API / session layer                 | out of scope — see README §Scope             |

`pkg/types` and `pkg/decimal` are the leaf packages everything else builds
on: the canonical spec-4 data-model structs, and exact-rational arithmetic
so money and epoch math never drift from floating-point rounding.

## The two Merkle trees, and why there are two

`pkg/state.MerkleTree` is a general-purpose SHA256 tree over 32-byte
`types.Hash` leaves — the public `StateRoot` a light client checks with
plain hashing (spec 7).

`pkg/zk.Tree` is a *separate* tree, native to the ZK circuit: MiMC over the
BN254 scalar field, because a SNARK circuit needs an R1CS-cheap hash to
prove Merkle membership in a reasonable number of constraints — SHA256
in-circuit would be orders of magnitude larger. This mirrors how production
shielded pools (e.g. Zcash Sapling) use a circuit-native hash for their note
commitment tree distinct from any general chain hashing.

`pkg/tx`'s Stage 1 (`pipeline.go`) is where the two are bridged: it takes
the `types.TransferPublicInputs` bound to a submitted transaction (which
carry `zk.FieldElement` values reinterpreted as `types.Hash` — both are
plain `[32]byte`, so the conversion is direct, via `zk.ToBytes32` /
`zk.FieldElementFromBytes32`) and verifies the Groth16 proof against them.
A production deployment would keep `pkg/zk.Tree` as the authoritative
note-commitment accumulator (rebuilt/updated at Stage 4, per spec 7) and
mirror its root into the public `StateRoot` structure for light-client
proofs; this reference implementation appends output commitments to both
trees at Stage 4 without yet reconciling them into a single canonical root
sequence — a bookkeeping unification, not a cryptographic one.

## Deliberate deviations from a literal spec reading

Each of these is called out at its point of implementation in code; they're
collected here for visibility.

1. **Revolver unfair-insert (spec 5.4.1 / 19.2).** A literal reading of the
   given pseudocode — re-checking `queue.Contains(addr)` before every one
   of the five inserts — makes positions 2 through 5 permanently dead code,
   since the very first insert makes `Contains` true for the rest of that
   same call. That silently defeats the mechanism the surrounding prose
   describes ("New joiners are inserted at several 'unfair' positions so no
   one parks at the front forever," spec section 1). `pkg/consensus.Revolver.InsertValidator`
   checks membership once at the start of a call (a duplicate join request
   is a no-op — spec 20's Testing Matrix: "duplicate ignored") and then
   scatters a genuinely new joiner into all five computed positions. See
   the doc comment on `InsertValidator` in `pkg/consensus/revolver.go`.

2. **`amount` as both a keyword and a value reference (spec 14.2 vs.
   14.5).** The core grammar reserves `AMOUNT` as the keyword introducing a
   tx's amount clause, but the canonical example in 14.5
   (`project_fee = amount * 0.05 to vault_address;`) uses bare `amount`
   inside the tx body to refer to that same clause's value — which the
   core grammar's `factor` rule cannot parse. `grammar/ShadowRustExt.g4`
   overrides `factor` (via ANTLR import, not by editing the pinned core
   file) to accept `AMOUNT` as an identifier-like reference, resolving the
   inconsistency the way spec 18.3's "internal review week: run the
   whitepaper queue-insert pseudocode through the toolchain, update this
   spec if it must change" step describes for exactly this kind of
   grammar/example mismatch.

3. **Bank withdrawal asymmetry factor (spec 11.2 / 19.4).** 19.4's
   pseudocode leaves the exact asymmetry formula as a comment
   (`// repaySFG includes asymmetry + ...`). `pkg/bank.Withdraw` implements
   it as `max(1, EntryPriceUSD / PriceNowUSD)`: a price drop since entry
   scales required repayment up proportionally (protecting the Bank's
   USD-denominated position), while a price rise leaves repayment at the
   entry-equivalent amount — matching both halves of 11.2's prose exactly.

## Known vet finding in generated code

`go vet ./...` reports "unreachable code" findings inside
`parser/shadowrustext_parser.go`. This file is generated by ANTLR
(`Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT
EDIT.`) — the pattern is ANTLR's standard error-recovery code shape and is
not present in any hand-written file in this repository. CI should exclude
`parser/` from `go vet` (see the command in the README's Testing section)
rather than patching generated output, which would be overwritten on the
next `antlr4` regeneration (spec 18.1: "Regenerating the parser is a CI
step").

## Production hardening still required before mainnet

This is a from-scratch, tested reference implementation, not an audited
production system. Before any real deployment:

- Replace `pkg/zk.Setup`'s development Groth16 setup with an audited
  multi-party trusted-setup ceremony (spec 23's own risk register calls
  for exactly this: "external audit, fuzz of the prover/verifier pair").
- Raise `pkg/zk.MerkleDepth` from its intentionally small reference value
  and re-run the ceremony for the new circuit.
- Wire `pkg/consensus`'s BFT quorum logic into live `StageVote` exchange
  in `cmd/node` (see README §Scope).
- Run the spec 18.6 hardening phase in full: 90% coverage gate, 24-hour
  fuzz runs on the pipeline and Bank math, TLA+ or equivalent on the
  revolver/megabatch invariants, and the 3-5 external audits spec 8.6 and
  23 call for.
