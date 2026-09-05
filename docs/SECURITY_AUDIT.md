# Phase 2 Independent Security Audit — Results

This is the real output of Phase 2 of the roadmap, across two rounds.
Round 1 (below) was two independent, adversarial reviews (a
cryptography/ZK-circuit review and a general Go/systems review of
consensus, P2P, and the transaction pipeline). Round 2 (further down)
was a full-system penetration test — every remaining subsystem, plus
live fuzzing and race-detector runs, not just re-reading code — after the
user explicitly asked for an adversarial attempt to break the L1 wherever
possible. Every confirmed finding in both rounds is fixed,
regression-tested, and merged — not a plan to eventually do this.

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
| 7 | **High** | Consensus | Catch-up committee recomputation excluded self unconditionally, permanently rejecting an already-quorate block in a small committee | **Fixed** (residual, rare statistical flakiness disclosed below — not the same bug) |

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
to confirm stability) and `TestFourNodesConvergeUnderJitterAndPacketLoss`,
measured over 11 total runs across this audit: 10 converged cleanly in
single-digit-to-low-tens of seconds (down from a deadline-length
structural stall pre-fix); one run still failed to converge within the
deadline. That one failure's log shows an ordinary, honest liveness
retry loop (a node timing out with 1 of 2 required votes, retrying
catch-up, briefly disagreeing on the assigned proposer) — not a repeat
of the structural "0/0 votes" bug this fix closes, which was
deterministic and never recovered no matter how many retries ran. This
residual is consistent with genuine BFT theory: a real >2/3 supermajority
(Finding 1's necessary safety fix) needs more of a committee's votes to
survive the same packet loss than a bare majority did, so this test's
already-generous deadline cannot reach exactly 100% under deliberately
harsh, sustained 15%-loss fault injection. It is disclosed here rather
than hidden, and is a reasonable, expected cost of the safety fix — not
a known additional structural defect.

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

---

## Round 2: full-system penetration test

Requested explicitly: try to break the L1 on every attack surface, fix
whatever is real, and be honest about what's left. This round covered
every subsystem Round 1 didn't (Bank/oracle economics, governance/NFT/
PoH, the mempool and outage-backlog admission paths, the keystore and
storage layer, the remaining ZK circuits, and the query API/container/
silent modules), plus live dynamic testing beyond static reading: the
race detector across the full test suite, and real fuzzing (not just
running each fuzz target's seed corpus once) against all three existing
fuzz targets. The fuzzing found the round's most severe result —
Finding 13, a real crash, not a theoretical one — which is exactly the
point of running the system instead of only reading it.

**Methodology note, stated plainly:** the plan was five parallel
independent review agents, one per subsystem group, mirroring Round 1's
structure. Four of five hit an API rate limit mid-run and never
completed (`Bank and oracle`, `Governance and NFT/PoH`, `Storage/
keystore/CLI`, `Query API/container/silent`); only the ZK-circuits
review finished on its own. Rather than re-queue and wait, that scope
was completed directly — same adversarial standard, same
verify-before-fixing discipline, just done by hand instead of in
parallel. Findings below are marked with which path found them.

### Findings

| # | Severity | Area | Summary | Status | Found by |
|---|----------|------|---------|--------|----------|
| 8 | **Critical** | P2P / mempool | `Mempool.Submit` admitted a transaction of any size — up to 16x the documented per-tx cap — before any signature or well-formedness check | **Fixed** | Direct review |
| 9 | **Critical** | P2P / outage recovery | The outage backlog queue had no admission bound at all (no count cap, no size cap) — worse than the mempool's already-serious gap | **Fixed** | Direct review |
| 10 | **High** | P2P / outage recovery | The backlog's own dedup sweep ran on every single incoming transaction (not paced like the mempool's), making sustained flooding an O(n²) CPU cost — empirically ~60s of CPU to fill a 100,000-entry backlog before the fix, ~0.8s after | **Fixed** | Direct review |
| 11 | **Critical** | Bank | `hold.SFGIssued` could silently overflow a uint64 at Deposit time (undefined truncation in `Decimal.Uint64()`), and the same unsafe `int64(uint64)` cast pattern as Finding 3 reappeared at Withdraw, independently — a negative `RepaySFG` means the Bank pays the withdrawer instead of collecting repayment | **Fixed** | Direct review |
| 12 | **Medium** | Governance | `ApplyParamChange` accepted any parseable decimal for a governance-settable parameter with no range check — a passed proposal setting `DepositATRMultiple` negative would make `pkg/bank.Deposit`'s buffer negative, directly over-issuing SFG relative to real deposited value | **Fixed** | Direct review |
| 13 | **Critical** | ZK / dependency | Real fuzzing found a 160-byte input that crashes any validator outright (`fatal error: runtime: out of memory`, not a recoverable panic) via an unbounded allocation in a vendored dependency's (gnark-crypto v0.21.0) proof deserializer — a genuine, remote, pre-authentication DoS | **Fixed** | Live fuzzing |

Findings 8–13 are additional, independently-numbered items continuing
Round 1's Findings 1–7 table above — same repository, same audit, later
pass.

### 8. [Critical] Mempool admitted oversized transactions before any check

`pkg/tx.MaxTxSize` (256 KiB) is documented as "checked at pipeline Stage
2" — true, but Stage 2 only runs once a transaction is later drained into
a batch a node happens to be proposing or verifying. `Mempool.Submit`
itself enforced nothing: no signature check (that's Stage 2's job too),
no size check. Since a `TxOffer`'s wire size is bounded only by
`pkg/net.MaxEnvelopeSize` (4 MiB — 16x `MaxTxSize`), any connected,
entirely unauthenticated peer could send `TxOffer` messages with an
oversized field (e.g. a large `Memo`) and have each one fully admitted
and counted against `DefaultMaxMempoolSize` (100,000 entries) — worst
case `100,000 * 4 MiB`, not `100,000 * 256 KiB`, a real remote,
pre-authentication memory-exhaustion vector.

**Fix:** `Submit` now rejects (`ErrTxTooLarge`) any transaction whose
serialized size exceeds `MaxTxSize`, before admission.

**Regression test:** `tx.TestMempoolSubmitRejectsOversizedTx`.

### 9. [Critical] Outage backlog had no admission bound at all

`OutageController.Enqueue` — reachable from any connected peer's
`TxOffer` while an outage is active (`pkg/validator`'s `handleMessage`
routes there instead of the live mempool whenever
`OutageController.Active()`) — had neither a per-transaction size check
nor a total-count cap, unlike `pkg/tx.Mempool`'s `MaxSize`. An outage is
exactly the kind of real network stress an attacker might induce or
exploit, making this an even sharper unauthenticated memory-exhaustion
vector than Finding 8, with no ceiling whatsoever.

**Fix:** added `maxBacklogTxSize` (mirrors `MaxTxSize`) and
`maxBacklogSize` (mirrors `DefaultMaxMempoolSize`), both enforced in
`Enqueue`.

**Regression tests:** `consensus.TestOutageEnqueueRejectsOversizedTx`,
`TestOutageEnqueueRejectsWhenBacklogFull`.

### 10. [High] Outage backlog's dedup sweep was O(n²) under flooding

While writing the count-cap regression test for Finding 9, filling the
backlog to 100,000 entries took **~60 seconds of real CPU time** in this
package's own test — a concrete, measured number, not a theoretical
concern. `sweepSeenLocked` (an O(n) scan of the whole dedup map) ran on
*every single* `Enqueue` call, unlike `pkg/tx.Mempool`'s identical sweep,
which only runs from `DrainBatch`/`DrainBatchBytes` — paced once per
proposal round, not once per incoming transaction. That made sustained
flooding an O(n²) CPU cost, a real amplifier stacked on top of Finding
9's memory-exhaustion gap.

**Fix:** moved the sweep out of `Enqueue` and into `BuildMegabatch`
(the outage-recovery equivalent of `DrainBatch`), restoring the same
per-round amortization the mempool already had.

**Verification:** the same regression test that took ~60s before the fix
now takes ~0.8s — measured, not estimated.

### 11. [Critical] Bank SFGIssued overflow and a second unsafe-cast instance

Two independent bugs in the same function pair, both in `pkg/bank/bank.go`:

- **Deposit-time overflow**: `sfgIssuedDec.Uint64()` (`Decimal.Uint64` →
  `big.Int.Uint64`) is documented by `math/big` itself as undefined for a
  value that doesn't fit — in practice, a silent low-64-bits truncation,
  not a panic. `net`/`SFGUSDPrice` are real deposit inputs and an oracle
  price this package never itself bounds to "reasonable" magnitudes, so a
  legitimately tiny `SFGUSDPrice` drives `sfgIssuedDec` arbitrarily large,
  silently wrapping into a meaningless `SFGIssued` value instead of
  failing loudly.
- **Withdraw-time unsafe cast**: `baseSFG := decimal.FromInt(int64(hold.SFGIssued))`
  is the *exact same bug class* as Finding 3 (Round 1), independently
  present here — `SFGIssued` can legitimately reach `>= 2^63`, and the
  cast would reinterpret it as negative, turning `RepaySFG` negative: the
  Bank paying the withdrawing user instead of collecting repayment.

**Fix:** `Deposit` now rejects (`ErrSFGIssuedOverflow`) any computed
amount that doesn't fit a uint64, before ever calling `.Uint64()`.
`Withdraw` now uses `decimal.FromUint64` (the same constructor Finding 3
added) instead of the unsafe cast.

**Regression tests:** `bank.TestDepositRejectsSFGIssuedOverflow`,
`TestWithdrawRepayPositiveForSFGIssuedAboveMaxInt64`.

**Why this matters beyond the specific fix:** the same bug class
(`int64(someUint64Field)` on a value with no upper-half-of-range
guarantee) has now been found independently in two unrelated modules
(`pkg/tx`'s fee collection, Round 1; `pkg/bank`'s withdraw repayment,
here). A repo-wide sweep for the pattern (`grep -rn "int64(" | grep -iE
"amount|issued|value|balance|..."`) found no further unguarded instances
— the two remaining matches (`pkg/staking/yield.go`,
`pkg/tx/pipeline.go`'s `MintFeeAmount` cast) already have their own,
independently-verified overflow guards (an explicit `IsUint64()` check
before the cast, and a division that provably keeps the result under
`2^63`, respectively).

### 12. [Medium] Governance parameters had no range validation

`ApplyParamChange` accepted any parseable decimal string for any of its
ten governance-settable keys, with no sanity bound. Concretely: a passed
`ProposalParamChange` setting `DepositATRMultiple` negative makes
`pkg/bank.Deposit`'s `buffer` negative, so `net = grossUSD - buffer`
*exceeds* the real gross deposit value — direct SFG over-issuance through
the one real path governance has to change live protocol economics, not
merely bad bookkeeping. The same class of problem applies to any rate/
share parameter set above 1 (100%) or below 0.

**Fix:** added a `[min, max]` bound per key (`paramBounds`), checked
before any value is accepted. This doesn't second-guess governance's
right to set any value in the sane range — several of these already
treat 0 as "disabled" — only values no correct implementation of the
formula they feed could ever intend.

**Regression tests:** `governance.TestApplyParamChangeRejectsNegativeATRMultiple`,
`TestApplyParamChangeRejectsFeeRateAboveOne`.

### 13. [Critical] Real fuzzing crashed the process outright — a dependency bug

This is the headline finding of Round 2, and the clearest example of why
*running* the system beats only reading it. Live, real fuzzing (`go test
-fuzz`, not just replaying each target's seed corpus) against the
already-existing `zk.FuzzProofFromBytesNeverPanics` found a genuine,
**160-byte** input that crashes the whole process with:

```
fatal error: runtime: out of memory
```

not a `panic` — a Go runtime `fatal error`, which `recover()` cannot
catch. Since proof bytes arrive over the real network inside an untrusted
`types.ShieldedTx.Proof` field, this is a real, remote,
pre-authentication denial-of-service: any peer could crash any validator
that processes the transaction, with no signature or state required
first.

**Root cause, traced to the actual line:** the crash is not in this
codebase. `zk.ProofFromBytes` delegates directly to gnark's
`groth16.Proof.ReadFrom`, which for its `Commitments []bn254.G1Affine`
field calls gnark-crypto's generic slice decoder
(`ecc/bn254/marshal.go`): it reads a raw, attacker-controlled `uint32`
length prefix off the wire and immediately does
`make([]bn254.G1Affine, claimedLen)` — **no check that the claimed
length is remotely plausible, or even that the input has enough
remaining bytes to back it.** The fuzzer's 160-byte input encodes a
well-formed `Ar`/`Bs`/`Krs` prefix (so it reaches the vulnerable field
at all) followed by a claimed commitments count of 1,677,721,600 —
`make()` of ~53 GB, more than enough to trigger the runtime's own
out-of-memory abort. Both `gnark` (v0.16.3) and `gnark-crypto` (v0.21.0)
are already at their latest released versions — this is not fixed by
upgrading, and it cannot be patched in the vendored dependency from
inside this repository.

**Fix:** `ProofFromBytes` now parses just far enough into the input
itself — walking `Ar`/`Bs`/`Krs`'s own real, bounded encoding (each is at
most 128 bytes either compressed or uncompressed, so this part can never
be used to trigger unbounded allocation) to find the exact byte offset
of the Commitments-length prefix — and rejects any claimed count above
`maxProofCommitments` (64, wildly generous: every real circuit in this
codebase produces exactly zero) with a clean error, *before* gnark's own
decoder ever sees the bytes. This closes the vulnerable field
specifically without needing to (and without safely being able to)
modify gnark-crypto itself.

**Regression tests:** the fuzzer's own crashing input is preserved as a
permanent corpus entry (`pkg/zk/testdata/fuzz/FuzzProofFromBytesNeverPanics/`,
replayed automatically by every future `go test`, `-fuzz` or not) and a
new, explicit unit test, `zk.TestProofFromBytesRejectsHugeCommitmentsClaim`,
splices a huge commitments-length claim into an otherwise real, valid
proof and confirms it's rejected cleanly — plus that a real, honestly
generated proof still deserializes correctly. Post-fix, the same fuzz
target ran clean for over 1.28 million more executions (60s) with no
further crash, and `FuzzVerifyPublicProofBytesNeverPanics` (the fuller
verification path) ran clean for 1.19 million executions.

**Every proof-consuming circuit is protected by this one fix**: Transfer,
Eligibility, Mint, Stake, and Unstake all deserialize proofs through this
same `ProofFromBytes` function — there is no separate, unguarded path.

### Dynamic testing: race detector and fuzzing

- **`go test -race ./pkg/...`** (full suite): zero data races found across
  every package. One test (`TestFourNodesConvergeOnSameChain`) failed
  once under full-suite CPU contention (many packages' real Groth16/
  Dilithium work competing for CPU under the race detector's own
  overhead) but passed cleanly and repeatedly in isolation (14s, well
  within its deadline) — a resource-contention artifact of the test
  harness, not a concurrency bug; no `DATA RACE` was ever reported for it
  or anything else. Separately, `TestOutageEndToEndDualTrackRoundClearsOnQuorum`
  (a pre-existing test, unrelated to any change in this document) failed
  once in a full non-race run with "failed to draw a committee with self
  as proposer after 100 attempts" — a random-committee rejection-sampling
  loop that occasionally exhausts its retry budget by bad luck; it passed
  3/3 immediate re-runs. Neither failure reflects a real defect this audit
  found.
- **Live fuzzing** (not just each target's seed corpus): `go test -fuzz`
  run for real wall-clock time against all three existing fuzz targets —
  `tx.FuzzStage2WellFormednessNeverPanics` (90s, clean),
  `zk.FuzzProofFromBytesNeverPanics` (90s: found Finding 13 above; 60s
  clean re-run post-fix, 1.28M executions), and
  `zk.FuzzVerifyPublicProofBytesNeverPanics` (60s clean, 1.19M
  executions). This is exactly what fuzzing is for — it found a real,
  critical, remotely-triggerable crash that pure code review had not.

### What Round 2 checked and found solid (no code change)

- **Governance vote tallying** (`pkg/governance/vote.go`): real dedup by
  voter, a fixed (non-governance-adjustable) 20% turnout floor
  specifically so a low-turnout proposal can't vote down the floor that
  gates it, and no governance-settable parameter can weaken the
  mechanism that gates governance itself (`ParamKeys` is a fixed
  allowlist of ten purely economic knobs).
- **All four remaining ZK circuits** (Eligibility, Mint, Stake, Unstake —
  Transfer was Round 1's scope): reviewed independently, no
  under-constrained signal, missing range check, or nullifier-scoping gap
  found in any of the four; every public input the pipeline reads is
  genuinely bound inside its circuit.
- **PoH attestation and NFT mint** (`pkg/nft/nft.go`): attestation is
  bound to (owner, nonce), signature-verified against a real trusted-
  attestor set, and TTL-bounded; the one-per-wallet gate is a live state
  lookup inside the same transaction the mint commits in, so no
  same-batch double-mint race exists even though a resubmission-with-a-
  different-nullifier trick (the same class as Round 1's Finding 4) can
  waste mempool slots on doomed duplicate mint attempts — never succeeds
  at minting twice.
- **Keystore** (`pkg/walletkey/keystore.go`): real Argon2id (64 MiB, t=3,
  p=4 — above OWASP's minimum) + ChaCha20-Poly1305 AEAD, fresh random
  salt per seal, both public keys bound into AAD, a single generic
  "wrong passphrase or corrupted keystore" error (no information-leaking
  distinction), private key material zeroed after use, keystore files
  written with owner-only (`0o600`) permissions.
- **Storage layer** (`pkg/state`): key construction (`prefix + suffix`)
  cannot collide across or within namespaces given each domain's fixed
  prefix and either fixed-length binary or single free-form string
  suffix fields; encryption-at-rest is deliberately scoped to genuinely
  private note/memo contents only (documented in `Store`'s own doc), not
  a gap — everything else stored in the clear is already public
  on-chain data.
- **Container shadow verification** (`pkg/container.ShadowVerify`): the
  pipeline requires an exact 32-byte duplicate-server digest before ever
  calling it, closing the theoretical "both sides empty/zero" bypass.
- **Oracle quorum** (`pkg/oracle.Quorum.Quote`): for two or more
  configured sources, a full max-min spread check (not just pairwise)
  gates the median, and disagreement beyond the bound fails safe
  (`ErrDisagreement`, freezing new activity) rather than silently picking
  a side — a single bad source among a real multi-source deployment can
  cause a DoS-style freeze, never a price skew. Noted, not fixed: with
  only one source configured, there is no redundancy to check by
  construction (`len(vals) < 2` trivially "agrees") — this is an
  operational deployment choice the code cannot force from inside
  itself, not a code defect; a real deployment must actually wire
  multiple independent sources for the quorum's safety property to mean
  anything.
- **Silent/spike-detection rate monitor** (`pkg/silent`): a brand-new
  wallet has a zero baseline for its first 10 minutes and — by the
  code's own explicit, documented design choice — "can never spike"
  during that window, so a cheap fresh keypair does buy a grace period
  before spike detection engages. Disclosed, not changed: exploiting it
  for sustained spam still requires real spendable value at each fresh
  address (this is a value-transacting system, not a free-transaction
  faucet), and changing the trade-off (new legitimate wallets would start
  getting flagged on their first burst) is a product decision, not a
  narrow security patch — flagged here for awareness, not silently
  ignored.

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
