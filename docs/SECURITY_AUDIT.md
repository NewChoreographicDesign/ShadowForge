# Phase 2 Independent Security Audit — Results

This is the real output of Phase 2 of the roadmap: two independent,
adversarial reviews (a cryptography/ZK-circuit review and a general
Go/systems review of consensus, P2P, and the transaction pipeline),
cross-checked against the actual source by direct manual reading, with
every confirmed finding fixed, regression-tested, and merged — not a
plan to eventually do this.

**What this is not:** a paid, external, third-party audit firm engagement.
The roadmap itself says this plainly — "An internal STRIDE pass is real
work, but it isn't a substitute for a paid, adversarial outside review."
This document is the internal half of Phase 2 (scoping the two review
angles, doing the work, fixing what was found, publishing it); engaging a
real external firm on the ZK circuit / Dilithium path and on consensus/P2P
remains open, and is the natural next step before mainnet.

## Scope

Two independent review passes, run in parallel with no shared context, each
instructed to find concrete, exploitable bugs only — no filler findings,
no invented issues to pad a report — and each cross-verified by hand
against the real source before anything below was accepted as genuine:

1. **Cryptography / ZK-circuit review** — `pkg/zk`'s Groth16 shielded-transfer
   circuit and Merkle tree, and the Dilithium (post-quantum) signing path
   across `pkg/crypto`, `pkg/tx`, `pkg/walletkey`, `pkg/validator`.
2. **Go/systems review** — `pkg/consensus` (committee assignment, BFT quorum
   math), `pkg/validator` (the propose/vote/commit state machine, catch-up
   sync, sentinel/outage handling), `pkg/net` (libp2p transport, framing,
   rate limiting), `pkg/chain` (block append and vote re-verification).

## Findings

| # | Severity | Area | Summary | Status |
|---|----------|------|---------|--------|
| 1 | **Critical** | Consensus | BFT quorum was a simple majority, not a real supermajority — inconsistent with this codebase's own claimed 1/3 Byzantine fault tolerance | **Fixed** |
| 2 | **Critical** | P2P / identity | Heartbeat identity was unauthenticated — any peer could hijack an already-online validator's identity | **Fixed** |
| 3 | **High** | Tx pipeline | `uint64` → `int64` cast on a ZK-proof-bound fee amount could silently corrupt Vault accounting with a negative value | **Fixed** |
| 4 | **Medium** | Tx pipeline | TxID/signature didn't commit to the real nullifier/commitments used for double-spend tracking, enabling mempool-dedup bypass | **Fixed** |
| 5 | **Medium** | P2P | `StageVote` replay could grow a round's vote list unboundedly (memory/CPU DoS) | **Fixed** |
| 6 | **Low/Medium** | P2P | `BlockRequest` triggered real per-request disk I/O with no rate limiting | **Fixed** |
| 7 | **High** | Consensus | Catch-up committee recomputation excluded self unconditionally, permanently rejecting an already-quorate block in a small committee | **Fixed** |

Everything marked Fixed is backed by a new, real regression test that
fails against the pre-fix code and passes against the fix (not merely
"doesn't crash") — same standard the rest of this repository already
holds itself to (see `docs/TESTING.md`).

---

### 1. [Critical] BFT quorum was a simple majority, not a real supermajority

`pkg/consensus/bft.go`'s `BFTQuorumMet` implemented spec 5.7's literal
wording — "a majority ... with one validator per stage that is 3 of 5;
with two per stage that is 6 of 10" — as `votes*2 > assigned`. Right next
to it, `BFTFaultTolerance` claims the protocol "tolerates up to one third
faulty nodes." Those two claims are mutually inconsistent: classical BFT
safety requires any two conflicting quorums to overlap by more votes than
the tolerated fault count, which for `f = assigned/3` requires a real
**>2/3 supermajority**, not a bare majority.

Concretely, with a 5-validator committee and `BFTFaultTolerance(5) == 1`:
two honest validators vote for candidate A, two different honest
validators vote for candidate B, and the one Byzantine (equivocating)
validator signs both. That's exactly 1 Byzantine validator among 5 — the
precise count this codebase claims to tolerate — yet the old rule let
**both** A and B independently reach "3 of 5" quorum: two conflicting
blocks finalized at the same height, a real double-finalization safety
violation, not a liveness nitpick.

**Fix:** `BFTQuorumMet` now requires `votes*3 > assigned*2`, the standard
BFT supermajority, consistent with `BFTFaultTolerance`'s own `f =
assigned/3` for every committee size this codebase produces (2, 4, 5, 10,
15).

**Regression tests:**
`consensus.TestBFTQuorumUnsafeAgainstClaimedFaultTolerance` reproduces the
exact double-finalization scenario above and fails against the pre-fix
code; `TestBFTQuorumOneValidatorPerStage` /
`TestBFTQuorumTwoValidatorsPerStage` / `TestTallyVotesReachesQuorum` were
updated to the real, safe thresholds (4-of-5, 7-of-10).

**Ripple effects fixed:** several existing tests hand-constructed blocks
with exactly the old majority vote count (`pkg/chain/chain_test.go`,
`pkg/query/server_test.go`, `pkg/queryclient/client_test.go`) and needed
updating to the real safe threshold — done, not weakened.

**A second, related gap this fix surfaced:** raising the quorum bar
exposed a latent bug in catch-up/replay committee recomputation — a node
adopting a block it did not itself vote on (an announce it lost the race
on, or multi-block catch-up) recomputed "the committee" from its *own
current* online view, which always includes itself (every node
self-registers as online at construction). For a newly-joined node
replaying old blocks, that inflates the quorum denominator with an
identity that provably cast no vote in the round being replayed, which
the old, looser majority threshold tolerated but the correct supermajority
does not. This is Finding 7 below; fixing it required more care than a
blanket exclusion (see that finding for why).

---

### 2. [Critical] Heartbeat identity was unauthenticated

`pkg/validator/consensus.go`'s `MsgHeartbeat` handler took a peer's
claimed `NFT` (identity) and `PubKey` and recorded them verbatim via
`recordOnline`, with no check that `NFT` was actually derived from
`PubKey` — even though `types.NFTID(types.SumHash(pk))` is exactly how
every node computes its *own* identity. `pubKeyLookup`, which both
`handleStageVote` and `chain.Append` rely on for real signature
verification, just returns whatever key is currently recorded for an
identity.

Concretely: an attacker sends a heartbeat with `NFT = <a real,
already-online validator's ID>` and `PubKey = <the attacker's own key>`.
Every subsequent `StageVote` the attacker signs with their own key but
labels with the victim's identity now verifies successfully and counts
toward quorum as that validator — full vote forgery/impersonation, not
just an unregistered-Sybil nuisance.

**Fix:** the heartbeat handler now rejects any heartbeat where `hb.NFT !=
types.NFTID(types.SumHash(hb.PubKey))`, before ever recording it.

**Regression tests:** `validator.TestHeartbeatRejectsIdentityHijack` (the
attack above, now dropped) and `TestHeartbeatAcceptsSelfConsistentIdentity`
(the legitimate case still works).

---

### 3. [High] Fee-amount overflow could corrupt Vault accounting

`TransferPublicInputs.FeeAmount` is a `uint64`, range-constrained only to
the ZK circuit's own 64-bit domain (`pkg/zk/circuit.go`'s `valueBits`), so
a real, provable transfer can legitimately carry a fee `>= 2^63`.
`pkg/tx/pipeline.go`'s `stage5PlaceFinal` routed it into the Vault via
`decimal.FromInt(int64(t.TransferPublicInputs.FeeAmount))` — casting a
value `>= 2^63` to `int64` reinterprets the top bit as a sign bit,
producing a large **negative** `Decimal`. `Vault.CollectFee` has no
non-negativity check, so every reward/burn pool
(`EpochBonusPool`/`AuditPool`/`RemainderPool`/`BurnedTotal`) would be
*decremented* instead of credited — real, permanent accounting corruption
reachable by anyone who can construct a transfer with a large enough fee
(e.g. from a large governance-minted note).

**Fix:** added `decimal.FromUint64`, which never round-trips through a
signed type, and switched the one real vulnerable call site to use it.

**Regression tests:**
`decimal.TestFromUint64PreservesMagnitudeAboveMaxInt64` (unit-level) and
`tx.TestPipelineCommittedTransferWithFeeAboveMaxInt64CreditsVaultPositively`
(end-to-end with a real Groth16 proof carrying a fee of `2^63 + 12345`,
asserting every Vault pool ends up positive).

---

### 4. [Medium] TxID didn't commit to the real nullifier/commitments

`types.ShieldedTx`'s own doc says the top-level `Nullifier`/`Commitments`
fields must always equal `TransferPublicInputs.Nullifiers[0]`/`.OutCommits`,
but nothing enforced that. Since `ComputeTxID` hashes the top-level fields
(not `TransferPublicInputs` directly), and actual double-spend tracking
(`MarkNullifierSpent`) correctly uses `TransferPublicInputs` — this was
never a fund-theft or double-spend hole — a key-holder could still wrap
one real, already-proven `(Proof, TransferPublicInputs)` pair in unlimited
distinct, individually-valid `(TxID, Sig)` combinations by re-signing over
a different top-level `Nullifier`/`Commitments`, defeating
`Mempool.Submit`'s TxID-keyed duplicate-transaction rejection for what is
really the same transfer (a gossip-amplification / mempool-bloat vector).

**Fix:** Stage 2 (`stage2TxOffer`) now rejects any `Kind Transfer` whose
top-level `Nullifier`/`Commitments` disagree with
`TransferPublicInputs.Nullifiers[0]`/`.OutCommits`.

**Regression tests:**
`tx.TestPipelineTransferRejectsNullifierMismatchedToPublicInputs` and
`TestPipelineTransferRejectsCommitmentsMismatchedToPublicInputs`.

---

### 5. [Medium] Unbounded StageVote replay

`StageVote` isn't one of `pkg/net`'s rate-limited message types (only
`Heartbeat`/`TxOffer` are, per spec 6), and `handleStageVote` appended
every signature-valid vote to the round's vote list with no check that the
same validator had already voted. `consensus.TallyVotes` already dedupes
by validator for *counting* quorum, but nothing stopped the underlying
slice — and the CPU cost of re-verifying and re-tallying every replay —
from growing unbounded as a peer replayed its own already-broadcast vote.

**Fix:** `handleStageVote` now drops a validator's second vote for the
same round before it's ever appended.

**Regression test:** `validator.TestHandleStageVoteDropsDuplicateFromSameValidator`.

---

### 6. [Low/Medium] BlockRequest had no rate limiting

Unlike every other unthrottled message type, `BlockRequest` triggers real,
synchronous disk I/O (`handleBlockRequest` reads up to `MaxCatchUpBlocks`
blocks from the store per request) — a peer flooding it was a cheap
I/O-amplification DoS.

**Fix:** `pkg/net.RateLimiter.Allow` now also throttles `MsgBlockRequest`,
using the same token bucket as `Heartbeat`/`TxOffer`.

**Regression test:** `net.TestRateLimiterThrottlesBlockRequest`.

---

### 7. [High] Catch-up committee recomputation excluded self unconditionally

Finding 1's supermajority fix requires an accurate committee for every
block a node independently re-verifies, including one it adopts via
`BlockAnnounce` or multi-block catch-up rather than voting on live. The
first fix attempt excluded this node's own identity from the recomputed
online set unconditionally, reasoning that a node adopting someone else's
already-decided block never itself contributed a counted vote. That's
true for genuine multi-block catch-up (a node replaying heights from
before it ever joined the network provably wasn't part of those
committees), but **wrong** for the ordinary single-block announce path: a
node adopting the very next height was almost certainly online and a real
candidate committee member for that exact round, just too slow to reach
local quorum itself.

In a small committee (this repository's own four-node integration and
load tests run with as few as two real validators), unconditionally
excluding self could shrink the recomputed committee below
`consensus.MinCommitteeSize`, producing an empty committee and a
permanent, structural — not merely probabilistic — rejection ("0/0 votes,
quorum not met") of an already-legitimately-quorate block on every single
retry, no matter how many. This was first mistaken for ordinary
packet-loss flakiness while hardening `pkg/validator/jitter_test.go`
against Finding 1's stricter threshold; once traced to its real cause, the
"more retries" mitigation added along the way (`Node.highestAnnounced` +
`sweepTimeouts`, which proactively retries real catch-up
(`requestCatchUp`/`handleBlockResponse`) independent of any per-round
timeout) remains a genuine, useful robustness improvement in its own
right, but the root cause was this miscount, not insufficient retrying.

**Fix:** `tryAdoptBlockLocked` now takes an explicit
`excludeSelfFromCommittee` flag: `false` from the single-block announce
path (`handleBlockAnnounce`), `true` from the multi-block catch-up path
(`handleBlockResponse`) only when a response actually carries more than
one block — the real signal that this node was behind for a genuine
stretch, not just an ordinary one-block-missed retry.

**Regression tests:** `TestNodeCatchesUpAcrossMultipleBlocks` (the
original multi-block scenario this exclusion exists for, run repeatedly
to confirm stability) and `TestFourNodesConvergeUnderJitterAndPacketLoss`
(six consecutive clean runs post-fix, converging in single-digit-to-low-
tens of seconds — down from a deadline-length structural stall pre-fix).

## What was checked and found solid

Beyond the findings above, both reviews spent real time confirming these
hold:

- The ZK circuit's five spec-8.1 constraints (Merkle membership, note-opening
  knowledge, nullifier derivation, value conservation with 64-bit range
  checks against field-modulus wraparound, and commitment well-formedness)
  are all genuinely constrained — no under-constrained signal was found.
- The Merkle tree's sparse/zero-hash-shortcut rewrite produces
  byte-identical left/right sibling selection to gnark's own
  `merkle.VerifyProof`, checked against the actual dependency source.
- Every signature-gated action (transaction authorization, stage votes, PoH
  attestation) fails closed on a verify error, never defaults to
  "verified."
- `pkg/chain.Append` independently re-verifies every vote's signature and
  committee membership — it never trusts an aggregate count.
- Catch-up replay independently replays and re-verifies every block
  exactly like a live announce; a peer cannot shortcut verification by
  answering a `BlockRequest`.
- `AssignCommittee`/epoch-boundary math are deterministic, pure functions
  with no internal bias or nondeterminism.
- Nullifier reservation/release atomicity across pipeline stages is
  correct, including under a forced later-stage failure.

## Running the audit's own regression tests

```sh
go test ./pkg/consensus/... -run TestBFTQuorum -v
go test ./pkg/validator/... -run 'TestHeartbeat|TestHandleStageVoteDropsDuplicate|TestNodeCatchesUpAcrossMultipleBlocks' -v
go test ./pkg/tx/... -run 'TestPipelineTransferRejects|TestPipelineCommittedTransferWithFeeAboveMaxInt64' -v
go test ./pkg/decimal/... -run TestFromUint64 -v
go test ./pkg/net/... -run TestRateLimiterThrottlesBlockRequest -v
```

## Next steps (per the roadmap)

1. Engage a real, paid external firm on the two angles this internal pass
   covered — the ZK circuit/Dilithium path, and consensus/P2P — per the
   roadmap's own framing; this document is the internal half of Phase 2,
   not a substitute for that engagement.
2. Budget real time for a second look once an external firm's report
   lands, exactly as the roadmap says — remediation is not a one-round
   activity.
3. Connection-level resilience (proactive reconnection/redial when a peer
   goes quiet, beyond the committee-recomputation fix in Finding 7) is
   still worth real, dedicated attention before Phase 3's public testnet,
   where uncontrolled real-world network conditions will exercise
   `pkg/net` far more than any local test does — nothing concrete was
   found wrong here during this pass, but it also wasn't the focus either
   review scoped in on.
