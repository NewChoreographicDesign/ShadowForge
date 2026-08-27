# ShadowForge L1 Master Specification (source)

_Verbatim text extracted from the uploaded master specification docx that this repository implements. Reproduced here per the spec's own repository layout (18.1: "docs/ // this specification") so implementers have the single source of truth alongside the code. Table structure and page layout from the original .docx are not preserved; content is._

---

Table of Contents


If the table of contents does not show page numbers immediately, open the document in Word and update fields (right-click the TOC → Update Field). Heading styles are already marked for levels 1–3.
SHADOWFORGE
Layer 1 Blockchain
Complete Master Specification
Architecture, Mechanics, Flows, Algorithms, and Implementation Blueprint
Field
Value
Document version
3.0 — Unified Master Spec
Compiled
27 August 2026
Sources reconciled
Whitepaper, Spec V1.0, Layout V1.0, High TPS Spec V2.0, Wallet Design, Enterprise Adaptation Plan, ShadowRust grammar and structure, ShadeLang research, Development Plan V2, Success Metrics, Phase 0 nano-steps, TX/NFT parse flowchart, outage and migration flowcharts
Purpose
Single source of truth so a team that has never seen the project can understand it and implement it from scratch
Status
Design-complete specification. Phase 1 planning items (spec unification and toolchain) treated as complete in the source plans. Implementation of ShadowRust MVP and L1 core is the next build work.

How this document was assembled
Where source documents disagreed, the later corrected text wins. Spec V1.0 bank correction (volatiles convert to SFG; stables later) overrides earlier “stablecoin-only L1” wording. ShadowRust-on-Go is the L1 backend language, not raw Rust. Success Metrics Year-1 TPS targets are the official launch bar; High TPS Spec describes the scaling architecture that is built so those numbers can grow. Formulas, queue positions, ATR multipliers, vault splits, and the five validation stages are taken from the latest explicit numbers in Spec V1.0, the whitepaper, and the detailed breakdown.
1. What ShadowForge Is — Plain Language
Imagine a public computer that many independent machines run together. That computer keeps a shared ledger of who owns what and which contracts ran. Today most such computers (blockchains) have three problems. First, every payment is visible, so anyone can watch your balances and counterparties. Second, the people who keep the ledger honest are usually those who already have a lot of money staked, which concentrates power. Third, moving Bitcoin or Ether onto a new chain dumps their price swings into that chain’s economy.
ShadowForge is a new Layer 1 blockchain built to fix those three problems at once. “Layer 1” means it is its own base network, not a side-chain that inherits another chain’s rules. It is designed from scratch. It does not fork Solana, Ethereum, or Cosmos.
Privacy is the default, not an add-on. When you send tokens, the network can prove the send is valid without publishing who sent it, who received it, or how much moved. Wallets create throw-away “mirror” addresses for each session and burn them afterward so chain analysis cannot stitch sessions together. If a tax office or auditor later needs a specific payment, you can export a reveal key for that payment only.
Anyone can become a validator without buying a huge stake. You mint a free soulbound NFT (one per wallet, with anti-bot checks). Turning validation on in the Creation App puts you into a rotating queue called the revolver. The network assigns work from the front of that queue. New joiners are inserted at several “unfair” positions so no one parks at the front forever. If almost everyone goes offline, ten protocol-run sentinel nodes take over so the chain does not halt.
You cannot dump raw Bitcoin volatility onto the ledger. The Bank is the on-ramp. You deposit a volatile asset. The Bank prices it with oracles, subtracts a volatility buffer equal to 2.5 times the current Average True Range, takes a 0.1 percent fee, and issues SFG (the native token) into your shielded wallet. When you leave, the Bank keeps 1.5 times the average ATR over your holding period and refunds any unused part of the original buffer, never less than zero. All in-network activity is in SFG. Direct stablecoin rails for businesses are a later expansion, not the launch path.
Developers who are not systems programmers are supposed to be able to build on it. ShadeLang is a visual and low-code language in the Creation App. Blocks and templates generate ShadowRust, a small domain-specific language. ShadowRust is parsed with ANTLR and compiled to ordinary Go, with zero-knowledge circuits, mutexes, and overflow checks injected automatically. The L1 node software itself is that generated Go, plus a small amount of hand-written Go for networking and storage.
The economic core is the ShadowVault. Every fee goes there. Governance, not a founding team, allocates it: roughly 20 percent epoch bonuses for validators, 10 percent burns, 10 percent audits and bounties, the rest grants and buybacks. Bank buffer yield is split 50/50 between SFG buybacks and community airdrops.
1.1 Who it is for
Users who want payments and balances that are private by default but can be disclosed on purpose.
People who want to help run a network without locking up large amounts of capital.
Beginner developers who want to ship a dApp without fighting a borrow checker or a 20-minute compile.
Businesses that want to replace departmental servers with a compact L1 container, department by department, without a big-bang cutover.
The existing crypto economy, which reaches ShadowForge only through the Bank so volatility stays outside the ledger.
1.2 What it is not
Not a Solana or Ethereum fork. No inherited runtime, no shared validator set.
Not a proof-of-work chain and not a coin-weighted proof-of-stake chain.
Not an L2. High throughput is achieved on L1 with pipelines, shards, staggered circuits, and enterprise containers so liquidity is not split across rollups.
Not a fully anonymous mixer with no compliance path. Reveal keys and optional KYC oracles exist for high-value and regulated flows.
1.3 One-sentence operating model
Users hold shielded SFG; NFT holders rotate through a revolver queue to validate five-stage batched transactions under exponentially lengthening epochs; sentinels and megabatches keep the chain alive through outages; the Bank converts external volatility into buffered SFG; ShadeLang and ShadowRust compile safe Go; the Vault recycles fees into scarcity, security, and rewards.
2. Design Principles and Non-Negotiables
Privacy by default. A transaction that does not produce a zero-knowledge proof of a valid state transition does not enter the pipeline.
Authority is identity-gated, not capital-gated. The NFT is the validator credential. Trust Points from uptime change queue priority; they do not replace the NFT.
Safety is injected, not requested. ShadowRust code generation wraps transfers in circuits, wraps shared state in mutexes or channels, and rejects overflow-unsafe arithmetic.
Volatility stays at the edge. L1 balances are SFG. External assets touch the chain only through the Bank.
Resilience over raw speed. A correct megabatch after an outage is more important than a peak TPS number. Sentinels exist so “too few validators” is not a halt condition.
Modularity. Network, consensus, state, transaction pipeline, bank, wallet, and app are separate modules with explicit interfaces.
Human-verified enterprise migration. No fully autonomous server rewrite. Engineers approve each departmental shadow before cutover.
Quantum readiness. Dilithium (or an equivalent NIST PQC signature) is the default signature scheme for stages and proofs. Classical signatures may exist only as a temporary dual-sign path during migration.
3. System Overview
3.1 Layer map
From the outside in:
Client layer — Flutter Creation App and Flutter/Rust wallet. ShadeLang editor (Blockly + Monaco). Hardware wallet passthrough.
API / session layer — gRPC and WebSockets to nodes. One-hour session tokens. Optional 2FA. Tor-capable transport.
Application layer — NFT contracts, Bank, Vault, governance votes, ShadeLang deploy pipeline.
Transaction layer — mempool, 1-second batcher, five-stage pipeline, ZKP verify, atomic commit.
Consensus layer — epoch clock, revolver queue, BFT vote, sentinel activation, megabatch / dual-track recovery.
State layer — encrypted account map, Merkle trees, container subspaces for enterprises.
Network layer — libp2p-style P2P, Noise encryption, DHT discovery, heartbeats, rate limits.
Tooling layer — ShadowRust grammar, ANTLR parser, interpreter, Go code generator, gnark circuits.
3.2 Runtime topology
Public validators: commodity machines or home nodes run by NFT holders who toggled Validate.
Sentinel validators: ten protocol-operated nodes that stay dark until online validators drop below ten.
Enterprise L1 containers: mini-PCs on a business site. They run a shielded subspace, process internal chained transactions, and periodically sync mega-batches into the public revolver.
Bank custody nodes: multi-chain wallets plus oracle clients. They are not consensus validators by default.
ShadeLang compile service: takes DSL, returns linter results, sandbox traces, or a deployable artifact.
3.3 Canonical technology stack
Concern
Choice
Why
L1 node language
Go (1.21+)
Seconds-scale compiles, goroutines for pipeline stages, Cosmos-like ops familiarity
L1 domain language
ShadowRust (ANTLR 4 → Go)
Rust-like structure without Cargo/borrow-checker pain; auto-injects safety
Parser generator
ANTLR 4.13.x + Go runtime
Visitor/AST, dialect imports, GUI parse trees for tests
Zero-knowledge
gnark (Go)
Native to the Go node; Groth16 / Plonk style circuits for shielded transfers
Post-quantum signatures
Dilithium (community Go / pqcrypto)
Stage signatures and wallet proofs
State store
Badger (embedded KV)
Go-native, can run in-memory for tests, encryption wrapper in front
P2P
libp2p + Noise
Encrypted channels, DHT, stream mux
Wallet crypto core
Rust (gnark/Halo2 bindings, sodiumoxide, pqcrypto)
Memory-safe key and circuit work closest to the user
Wallet / App UI
Flutter
One codebase for iOS, Android, web, desktop
Visual DSL
Blockly + Monaco
No-code and low-code in the same app
Formal / model checking
TLA+ on consensus and queue
BFT and revolver invariants
Containers / isolation
Docker, optional QEMU / Windows Sandbox for wallet virtual state
Enterprise replica and malware isolation
Oracles
Redundant price + ATR feeds (Chainlink-class + fallback)
Bank math and green-energy attestations
Optional KYC
Worldcoin-class PoH oracle, toggle per threshold
High-value Bank and mint anti-sybil
3.4 Official performance and health targets
Two layers of numbers exist in the source corpus and both are kept. Launch metrics are the Success Metrics document. The High TPS specification is the scaling architecture that must be implemented so the network is not boxed into those Year-1 numbers forever.
Metric
Testnet
Mainnet Year 1
Scaling architecture target
Sustained TPS
500–1,000
2,000–5,000
1M+ average design capacity via shards + circuits + containers
End-to-end latency
&lt; 10 s
3–5 s average
Keep ~5 s even under megabatch recovery (&lt; 30 s outage catch-up)
Uptime
&gt; 95%
&gt; 99.5%
&lt; 1% downtime; sentinels &lt; 5 activations / month
Batch cadence
1 s
1 s
1 s live; megabatch 10× on recovery
Pipeline stages
5
5
5, with 2–3 validators per stage above 200 / 300 online
Other Year-1 success bars from the metrics document: 100k–500k active shielded wallets; 5k–10k NFTs with 30–50% actively validating; 100k–500k transactions per day; TVL $100M–$500M; monthly SFG volatility under 20%; vault allocation efficiency above 80%; bank yields 2–5% APY on buffers; governance turnout above 20% of eligible NFTs; 200–500 dApps; 3–5 external audits before mainnet; zero major exploits; less than 5% of TVL at residual risk.
4. Data Models
Every implementer should treat the following structs as the canonical shapes. Field names may be camelCase in Go. Semantic meaning must not change.
4.1 Identifiers
Address: 32-byte shielded account identifier. Public explorers show only a truncated hash.
Mirror: ephemeral address derived as Hash(master_pubkey || session_nonce || timestamp). Burned at session end.
NFT ID: soulbound token id, one per wallet at mint. Traits are a key-value map.
TxID: Hash of the shielded transaction blob (proof + commitments + nullifier).
Block height: uint64, genesis = 0.
Epoch number: uint64, genesis epoch = 0.
4.2 Transaction
A user-facing transfer is never stored in cleartext. The persisted object is:
type ShieldedTx struct {
    TxID          Hash
    Nullifier     Hash          // prevents double-spend of the note
    Commitments   []Hash        // new notes created
    Proof         []byte        // gnark proof bytes
    FeeCommit     Hash          // fee paid to Vault, also shielded
    Memo          []byte        // optional encrypted memo for receiver
    Sig           DilithiumSig  // stage / wallet signature
    Kind          TxKind        // Transfer | Mint | Vote | BankDeposit | BankWithdraw | NFTTrait | ContainerSync
    StageHints    StageSet      // which of the 5 stages already passed (for pipeline retry)
    ContainerID   *ID           // nil unless originated in an enterprise container
}
TxKind determines which extra public inputs the circuit must bind. BankDeposit binds oracle price, ATR, and buffer. Vote binds proposal id and a yes/no commitment. NFTTrait binds the trait key and a delta commitment.
4.3 Block
type Block struct {
    Height        uint64
    Epoch         uint64
    PrevHash      Hash
    Timestamp     int64         // unix ms, proposer local, bounded by network skew rule
    Batch         []ShieldedTx  // one 1-second batch, or a megabatch on recovery
    TxRoot        Hash          // Merkle root of the batch
    StateRoot     Hash          // Merkle root after applying the batch
    DARoot        Hash          // data-availability commitments for shielded blobs
    Proposer      NFTID
    ProposerSig   DilithiumSig
    Votes         []Vote        // BFT signatures from assigned stage validators
    DualTrack     bool          // true if this block is the backlog track of a megabatch
}
4.4 Account / note state
The user-visible “balance” is a set of unspent shielded notes, not a transparent integer. State storage is an encrypted KV map keyed by note commitment. A Merkle tree over commitments is the public state root.
type Note struct {
    Commitment    Hash     // public
    Value         uint64   // private, inside the circuit
    OwnerPk       PubKey   // private
    Rho           []byte   // nullifier seed, private
    Asset         AssetID  // SFG at launch; other assets only inside Bank custody
}
type AccountMeta struct {
    NFT            *NFTID
    TrustPoints    uint64
    ValidateOn     bool
    MintOn         bool
    RevealKeyHash  *Hash   // exists if user registered a selective-disclosure capability
    DailyBankUsed  uint64  // USD-equivalent, reset at UTC midnight
}
4.5 NFT
type ValidatorNFT struct {
    ID          NFTID
    Owner       Address     // soulbound; transfer disabled until governance unlocks a trait
    MintedAt    uint64      // block height
    Traits      map[string]string  // e.g. badge=RecoveryHero, dept=Finance
    TP          uint64
    Slashed     bool
}
Department NFTs used in enterprise migration are the same type with extra traits (income, balance, stock, …). Trait updates are themselves shielded transactions of Kind NFTTrait and must pass the five-stage pipeline.
4.6 Revolver queue item
type QueueItem struct {
    NFT        NFTID
    Address    Address
    JoinedAt   int64
    LastBeat   int64
    CooldownUntil int64   // set to now+1h when a node goes offline
    TP         uint64
}
4.7 Bank hold
type BankHold struct {
    HoldID         Hash
    Owner          Address
    ExternalAsset  AssetID      // BTC, ETH, ...
    ExternalAmount Decimal
    EntryPriceUSD  Decimal      // oracle snapshot
    EntryATR       Decimal      // current ATR at deposit, USD
    EntryBuffer    Decimal      // 2.5 * EntryATR
    EntryFee       Decimal      // 0.1% of (gross - buffer)
    SFGIssued      uint64
    OpenedAt       int64
    DailySnapshots []ATRPoint   // used to compute average ATR over the hold
    Status         HoldStatus   // Open | Locked24h | Closing | Closed
    CycleCount30d  uint        // for &gt;3/month surcharge
}
5. Consensus
5.1 Why PoA with NFTs
Proof of work wastes energy and still centralizes around hardware. Coin-weighted proof of stake gives the already-rich more votes. ShadowForge uses Proof of Authority where the authority credential is a free, one-per-wallet NFT plus live heartbeats. Byzantine fault tolerance still applies: the assigned committee for a batch must reach majority agreement and the protocol tolerates up to one third faulty nodes among those who are currently online and assigned.
5.2 Epochs
Epochs are the slow clock of the chain. They govern validator rotation windows, mint finalization, and bonus distribution.
Duration formula: duration(epoch) = min(1 hour × 1.1^epoch_number, 1 year).
Epoch 0 lasts 1 hour.
Epoch 1 lasts 1.1 hours.
Epoch 10 lasts about 2.59 hours.
Epoch 50 lasts about 117 hours.
The cap is 8,760 hours (one non-leap year). After the cap is hit, every later epoch is one year.
A mint proposal is submitted at or after epoch start, accumulates private ZKP votes during the epoch, and finalizes only at epoch end. That is intentional friction. It makes spam mints expensive in time rather than only in fees.
Implementation: store GenesisTime. Current epoch is the largest n such that sum(duration(i) for i in 0..n-1) &lt;= now - GenesisTime. Use integer milliseconds and a decimal or rational 11/10 multiplier so floating error cannot skip an epoch.
5.3 The five-stage validation pipeline
Every transaction, including NFT trait updates produced by enterprise parsers, walks the same five stages. Stages are sequential for a given transaction and pipelined across the batch. Target latency under normal load is about five seconds: one second of batching plus roughly four seconds of stage work and commit.
Stage
Name
What is checked
What is written
1
Sender Leave
ZKP that the spent notes exist, are owned, and are not already nullified. Balance never revealed.
Nullifier reserved in a pending set (not yet finalized)
2
TX Offer
Well-formedness: kind, fee commitment, circuit public inputs, Dilithium signature, not expired.
Tx admitted to the working batch
3
Receiver Check
Receiver note parameters legal; optional compliance hook (KYC oracle flag if amount commitment is in a high-value class the user opted into); container routing if ContainerID set.
Receiver pending note
4
Send Exec
Atomic application of the state transition against a working copy of the Merkle tree. Bank and mint kinds run their extra math here.
Working state root candidate
5
Place Final
BFT votes from assigned validators; commit state root, DA root, burn ephemeral pipeline artifacts.
Block inclusion, pending → committed
Atomicity rule: if any stage fails, the whole transaction is rejected, pending nullifiers are released, and no trait or balance changes. This is the “atomic updates via 5-stage validation” path on the Parsing TXs/NFT flowchart. Shadow verification compares the parser’s expected outputs with the L1 container’s outputs before Stage 5 is allowed to commit an enterprise trait update.
5.3.1 Stage assignment
Fewer than or equal to 200 online validators: one validator popped from the revolver per stage (five distinct validators per batch if the queue is long enough).
More than 200: two validators per stage, batch split into two sub-batches.
More than 300: three validators per stage, three sub-batches.
A validator who finishes a stage successfully is pushed to the back of the revolver.
A validator who fails a stage (invalid signature, timeout) does not earn TP, may be cooldown’d, and is not pushed to the back until a later successful turn.
5.4 Revolver queue
The revolver is a global deque of online, Validate-toggled, non-cooldown NFT holders.
5.4.1 Join insertion (unfair positions)
When a validator comes online, do not only append. Insert the address at each of these indexes, skipping if already present:
positions := []int{ queue.Len(), 4, 10, 2, 7 }
for _, pos := range positions {
    idx := pos % (queue.Len() + 1)   // +1 so append is representable
    if !queue.Contains(addr) {
        queue.Insert(idx, addr)
    }
}
The first position is “last” (append). The others scatter the joiner through the current queue so a fresh node cannot monopolize the next five stages and so two joiners are unlikely to collide on the same stage of the same batch. Low Trust Point holders may have extra delay before the first insert (Sybil brake).
5.4.2 Heartbeats and cooldown
Heartbeat interval: 10 seconds over Noise-encrypted P2P.
Missed heartbeat window: treat offline after 3 missed beats (about 30 seconds).
Offline penalty: 1 hour cooldown before the NFT may re-enter the revolver.
TP: increment on successful stage work and on continuous uptime slices; decrement or freeze during cooldown.
5.5 Sentinels
If the number of online, non-cooldown validators in the revolver is strictly less than 10, the protocol activates 10 sentinel validators. Sentinels are protocol-run servers with well-known identities. They process stages exactly like NFT validators but their rewards go to the Vault rather than to a personal NFT. When the civilian queue recovers to 10 or more, sentinels withdraw in an orderly way after finishing the current batch. Sentinel activations are first-class metrics (Year-1 budget: fewer than 5 activations per month).
5.6 Outage handling
An outage is declared when heartbeats are missing from more than 50 percent of the last-known online set, or when stage timeouts cascade. The flowchart in Outage Handling.pdf is a linear recovery pipeline. Implement it as the following sequence:
Detect: heartbeat monitor raises OutageFlag.
Pause live batch admission. Incoming user transactions go to an on-disk backlog queue. Wallets may keep signing offline.
If civilian online count &lt; 10, activate sentinels.
Enterprise containers that cannot reach the public network flip to internal mode: 100 percent of their local validators process local traffic only.
On recovery (heartbeats restored from a majority), build megabatches of 10× normal batch size from the backlog.
Process each megabatch on dual tracks per stage: Track A takes new live traffic, Track B drains backlog. DualTrack=true on backlog blocks.
Silent transactions (see High TPS) may be injected by sentinels during desync to keep circuits warm and to absorb junk load.
Clear OutageFlag only after backlog depth is below a configured threshold and one clean dual-track cycle has committed.
Local container queues must stay under about 5 percent extra storage and must be pruned after sync. Megabatch catch-up target is under 30 seconds of additional user-visible latency.
5.7 BFT vote rule
For a batch to finalize at Stage 5, a majority of the validators assigned to that batch’s stages must sign the candidate state root. With one validator per stage that is 3 of 5. With two per stage that is 6 of 10. Invalid votes are slash proposals, not automatic burns; burning an NFT requires a Creation App governance vote.
6. Networking
Transport: libp2p with Noise XX (or equivalent Noise handshake). All consensus messages encrypted.
Discovery: bootstrap node list in genesis plus DHT. Enterprise containers may use private bootstrap plus a single public gateway.
Message types: Heartbeat, TxOffer, StageVote, BlockAnnounce, MegabatchPart, ContainerSync, SilentPad.
Rate limits: per-peer token bucket on Heartbeat and TxOffer. Exceeding the bucket drops the peer for a cooldown. This is the first DoS brake.
ShadowRust surface: a network { ... } statement compiles to a libp2p host with the above defaults injected.
Multi-node verification in Phase 2 of the development plan is a Docker compose of at least four nodes: two civilian validators, one sentinel candidate, one client wallet simulator.
7. State Management
State is an account/note model, not a UTXO-only model and not a transparent Ethereum-style account. Badger (or an equivalent embedded KV) stores:
commitment → encrypted note blob
nullifier → spent flag
nft id → ValidatorNFT
hold id → BankHold
proposal id → vote commitments
container id → subspace root
A Merkle tree is rebuilt or incrementally updated at Stage 4. The public block header carries StateRoot and DARoot. Encrypted blobs are the data-availability payload; light clients verify Merkle proofs against DARoot without seeing note contents.
ShadowRust state statements (store, load, update_trait) compile to Badger transactions wrapped in the same five-stage commit. Fuzz tests must include random interleavings of transfers, trait updates, and bank opens.
8. Privacy and Security
8.1 Shielded transfers
The transfer circuit (gnark) must prove, without revealing values or addresses:
Each spent note commitment exists in the Merkle tree at a private path.
The spender knows the opening (value, owner key, rho).
The nullifier is correctly derived from rho so the same note cannot be spent twice.
Sum(spent values) = sum(new note values) + fee.
New commitments are well-formed bindings of the claimed secret openings.
Explorers display TxID, timestamp, and kind. They do not display sender, receiver, or amount. Aggregate network statistics (count of TXs, approximate fee volume via fee commitments) are public.
8.2 Ephemeral mirrors
On wallet session start the client samples a nonce and derives mirror = Hash(master_pk || nonce || ts). Outbound transactions use the mirror as the network-visible routing handle. On session end the wallet:
Flushes pending signatures.
Submits a burn/abandon of the mirror (or simply never reuses it; the protocol treats unused mirrors as inert).
Optionally toggles a brief offline window so malware that needs live network to exfiltrate keys sees a disconnect. See Wallet section.
8.3 Reveal keys
A reveal key is a user-held secret that decrypts a chosen transaction memo and note opening. Export format is JSON with the TxID list and the decryption material. The chain never stores the reveal key. Wallets may register only a hash of a capability so an auditor can later demand a matching opening. This is how tax and enterprise audit work without a global back door.
8.4 Compliance hooks
Optional KYC via Worldcoin-class oracles for transactions or bank deposits above a governance-set threshold (documents mention $10k as a starting example for wallet KYC prompts).
Hashed IP fingerprints on Bank flows to flag cycling/arbitrage. Store H(ip || day_salt), never raw IP.
Governance can raise or lower thresholds. Default path remains private.
8.5 Post-quantum signatures
Dilithium signs: wallet authorizations, stage votes, block proposals, container sync aggregates. Circuit proofs remain SNARKs (classical assumptions) in v1; a later dialect can swap the proving system. Dual-sign (Dilithium + ed25519) is allowed only as a migration aid and must be scheduled for removal by governance.
8.6 STRIDE threat model (from Spec V1.0)
Category
Example threats
Required controls
Spoofing
Fake NFT, forged session token, spoofed oracle ATR
Soulbound NFT + PoH/CAPTCHA; time-bound session + 2FA; multi-oracle quorum
Tampering
Queue rewrite, ATR math edit, ShadeLang swap at compile
Merkle state; immutable generated Go; linter before compile; BFT on roots
Repudiation
Validator denies a proposal; user denies a deposit
Dilithium on every stage; TP logs; reveal keys for the parties who need them
Info disclosure
Shielded note leak; bank hold dump; forum identity join
Default ZK; hashed fingerprints; encrypted P2P; ephemeral mirrors
Denial of service
Queue flood, oracle DDoS, validator mass-offline
Rate limits; sentinels; megabatches; silent TXs; adaptive scaling
Elevation
User becomes validator without NFT; bank limit bypass
NFT gate; TP thresholds; KYC for raised limits; governance revocation
Residual risk target: under 5 percent of TVL exposed after mitigations. Vault pays for 3–5 external audits (Hacken-class) before mainnet and for ongoing bounties.
9. Tokenomics
9.1 SFG
Ticker: SFG.
Initial supply: 500,000,000. No founder pre-mine in the spec.
Inflation: 2–5 percent per year, set by governance, unlocked only against activity (transaction volume and validator participation).
Uses: gas, governance weight (via NFT + optional stake), Creation App access, Bank settlement.
9.2 Fees and ShadowVault
Every transaction fee, privacy premium, Bank entry/exit fee, and container-sync fee lands in the ShadowVault, a multi-signature treasury contract governed by NFT votes.
Slice
Share
Destination
Epoch bonuses
20%
Validators, weighted by Trust Points and uptime, paid at epoch boundaries
Burns
10%
Permanent supply reduction
Audits and bounties
10%
External firms and white-hats
Remainder
60%
Grants, infrastructure, green offsets, SFG buybacks
Bank buffer yield (after the Bank invests retained ATR buffers in low-risk venues) is separate: 50 percent SFG buybacks, 50 percent community airdrops.
9.3 Incentive table
Validate and Mint both toggled: +20 percent epoch bonus multiplier.
Only one toggle: −10 percent or a specialized badge (“Mint Master”, “Valor”) instead of the cash multiplier.
Long Bank hold: “Stable Saver” badge, 10 percent reduction on future ATR buffers.
Verified green hardware / renewable energy oracle: extra Trust Points.
Human-verified wallet sessions (mouse-tracking pass): small TP or queue-priority perk, optional.
10. NFTs and Validators
10.1 Free mint
User creates a wallet (see section 12).
User requests a micro-drop of SFG sufficient for one mint transaction (anti-bot gated).
Mint UI presents CAPTCHA and a proof-of-humanity challenge (Gitcoin Passport / Worldcoin-class).
Contract enforces one NFT per wallet. A second mint from the same owner key fails.
NFT is soulbound at mint. Trading stays disabled until a governance vote unlocks a transfer trait.
Launch giveaway: 500–1,000 NFTs via lottery to seed the first revolver. Year-1 target 5,000–10,000 NFTs with 30–50 percent actively validating.
10.2 Becoming a validator
In the Creation App the holder toggles Enable Validating. The node software then sends heartbeats and becomes eligible for unfair-position insert into the revolver. Enable Minting is a separate toggle for participating in epoch mint proposals. Both toggles together grant the +20 percent bonus.
10.3 Sybil and slashing
Trust Points accrue from uptime and successful stages. Low TP delays revolver insertion.
Malicious stage signatures or provable double proposals create a slash proposal.
Slash execution is a governance vote that burns or freezes the NFT. Automatic silent burns are not in spec.
10.4 Department NFTs
In the enterprise path, each business department maps to a soulbound NFT whose traits are the department’s live numbers (balance, stock, income). Human scripts plus ShadowRust parse server logs and emit trait-update transactions. The L1 container applies them through the same five stages. Shadow verification diffs the container output against the duplicate server before commit. See section 16.
11. The Bank
The Bank is the only supported way for volatile external assets to become in-network value. L1 itself does not hold BTC or ETH in user accounts. Those assets sit in the Bank’s multi-chain custody wallets. Users hold SFG notes.
11.1 Deposit
User sends external asset A of amount Q to the Bank multi-chain address, including a memo that binds their ShadowForge shielded address.
Bank reads price P and current ATR from a quorum of oracles. ATR is in USD.
GrossUSD = Q × P.
Buffer = 2.5 × current_ATR. (If ATR is published as a percent of price, convert to USD first. Specs treat ATR as a USD volatility charge on the position.)
If Buffer &gt;= GrossUSD the deposit is rejected; the asset is returned minus a dust network fee.
Net = GrossUSD − Buffer.
EntryFee = 0.001 × Net.
SFGIssued = (Net − EntryFee) / SFG_USD_price.
Bank opens a BankHold, logs timestamp, EntryATR, Buffer, fee, and starts daily ATR snapshots.
Bank emits a shielded BankDeposit transaction that credits SFGIssued to the user. This TX walks the five stages.
24-hour lock: Status = Locked24h. User cannot open a matching withdraw until the lock ends.
11.2 Withdrawal
User requests close of HoldID and offers SFG.
Bank reads current price P_now and computes average ATR over daily snapshots of the hold (AvgATR).
Retention = 1.5 × AvgATR.
Refund = max(0, EntryBuffer − Retention). Paid in SFG.
Asymmetry: if the external asset depreciated versus entry, the SFG the user must repay is increased so the Bank is not left short. If the asset appreciated, repayment is fixed at the entry-equivalent SFG (user does not receive extra external asset beyond what was deposited).
Slippage estimate 0.2–0.5 percent from a liquidity oracle, plus 0.1 percent exit fee, plus on-chain TX fees, are added to the SFG the user must deliver.
On confirmation the Bank releases the original asset minus the exit-fee equivalent and marks the hold Closed.
11.3 Safeguards
24-hour deposit lock.
More than 3 deposit/withdraw cycles in 30 days: +1 percent surcharge.
Hard cap $100,000 USD-equivalent per wallet per UTC day unless KYC raises the cap.
Oracle quorum; if oracles disagree beyond a bound, freeze new deposits and use last-good snapshots for open holds.
Hashed IP fingerprint to flag correlated abuse.
Governance override for insolvency or oracle catastrophe.
Refunds cannot go negative. Documents also mention an 80 percent-of-original-charge cap on refunds as a reserve-protection option; implement the cap as a governance parameter defaulting to 100 percent (i.e. only the max(0, ·) rule) unless voted down to 80 percent.
11.4 Yields and product surface
Buffers and fees are invested in low-risk stable venues. Profits: 50 percent buybacks, 50 percent airdrops. The Creation App shows a Bank portal with an estimator that previews 2.5× ATR on the way in and expected retention on the way out. “Stable Saver” badge after long holds reduces future buffers by 10 percent.
Direct stablecoin L1 balances for businesses are explicitly postponed. Do not implement transparent USDT accounts in v1.
12. Wallet
The wallet is the fortress in front of the private key. It is specified independently of the node and ships on a later Phase 3 track, but its interfaces must exist as mocks during Phase 2 TX work.
12.1 Stack
UI: Flutter, all platforms.
Crypto core: Rust. gnark or Halo2 wrappers, sodiumoxide, pqcrypto Dilithium, BIP-39/44.
Keys: OS secure enclave (Android Keystore, iOS Secure Enclave) plus encrypted container on desktop.
Network: gRPC / WebSocket to nodes, optional Tor. Offline-first signing.
Hardware: Ledger and Trezor Rust SDKs. A “Hardware Mode” toggle forces every signature through the device.
12.2 Required features
Default shielded send / receive. No transparent send button in the main path.
Ephemeral mirror lifecycle bound to the session.
Reveal-key export wizard.
Bank estimator and deposit/withdraw flows.
NFT mint wizard with CAPTCHA / PoH.
Validate / Mint toggles and ZKP vote ballots.
Offline disconnect: on start and on shutdown, disable the network for 5–10 seconds (WinINet / SCNetworkReachability / nmcli / airplane-mode intent) while keys load or mirrors burn. If the OS call fails, show a blocking prompt.
Virtual state: desktop build can launch inside Windows Sandbox, Flatpak, Docker, or QEMU with a disposable filesystem shredded on exit.
Mouse tracking: Flutter MouseRegion collects path, velocity, pause statistics. A small local model (tch / equivalent) scores human-likeness. Score under 80 percent triggers lockout or reCAPTCHA. Tracking stays on-device. It is optional and disclosed.
Session tokens last one hour. Optional 2FA / YubiKey.
Outage queue: if the node is unreachable, signed transactions sit locally and flush into megabatch recovery.
12.3 Wallet nano-plan (from Wallet Design)
Planning and wireframes, 1–2 weeks.
Key management and mirrors, 2–3 weeks.
TX builder, Bank math, offline sign, 2 weeks.
Flutter UI, wizards, offline disconnect, 3–4 weeks.
NFT, governance, mouse tracking, hardware mode, 2–3 weeks.
Fuzz, pentest, virtual-state isolation, 2–4 weeks.
Stores, signed updates, anonymous metrics, ongoing.
13. Creation App and ShadeLang
13.1 App
The Creation App is NFT-gated. On connect it scans for a valid NFT, issues a one-hour session token, and optionally asks for 2FA. Inside the app: mint escrow (10 percent fee to take tokens immediately, or 2 percent USDT-equivalent stake yield), private ZKP voting, Validate/Mint toggles, Bank portal, guilds, shielded forums, push notifications for epoch and queue events, and the ShadeLang studio.
13.2 ShadeLang studio — six frontend pieces
These exist to remove the Solana/Rust beginner wall documented in the ShadeLang research file: borrow-checker confusion, Cargo hell, long compiles, missing sandboxes, and no visual start.
Component
Job
Backend hook
Blockly visual builder
Drag TX / queue / mint / bank / container blocks
Exports ShadowRust text
Monaco textual editor
Low-code with autocomplete against the grammar
Same DSL string; stays in sync with Blockly via AST
Linter panel
Instant flags: unshielded TX, overflow, missing fee route
ShadowRust semantic analyzer API
Sandbox preview
Run 100 mock TXs, show latency and epoch clock
ShadowRust interpreter + mocked Badger
Template gallery
Shielded DEX, L2 scaffold, department NFT, Bank helper
JSON templates that expand through the compiler
Quest overlay
Gamified first-TX, first-vote, first-deploy
Writes TP / badge traits through the L1
13.3 End-to-end create-and-deploy flow
User picks a template or starts from an empty canvas.
Blockly and Monaco stay synchronized.
On every change the editor posts DSL to the linter. Errors appear inline.
Sandbox compiles to a temporary Go program or interprets the AST against mocks.
Deploy submits the generated artifact through a five-stage transaction of Kind appropriate to the template (often a contract-register or container-define).
Governance may require a vote before a high-privilege deploy (new container type, new Bank asset).
14. ShadowRust — Language and Compiler
ShadowRust is the backend domain-specific language of ShadowForge. It is embedded in Go: there is no separate VM in production. The interpreter exists only for sandboxing and tests. Production artifacts are ordinary Go packages generated by a visitor.
14.1 Pipeline
ShadeLang or a human emits a .sr file.
ANTLR lexer/parser builds a parse tree.
Visitor builds a typed AST, then an IR.
Semantic analyzer rejects unsafe programs (no ZKP on tx, mutable shared state without a lock, non-numeric amount, missing fee route on value-moving statements).
Optional IR transforms (overflow-safe math, dialect rewrites, version migrations).
Either interpret against mocks, or generate Go with gnark circuits, Dilithium signatures, Badger writes, and goroutine/channel patterns.
go test / go build the generated package. Deploy the binary to a node or a container.
14.2 Canonical grammar (v1, from shadowrust.g4.txt)
This is the grammar that Phase 1 must parse. High TPS and enterprise work extend it with container, shard, async_stagger, resilience, and network statements. Extensions must be added as ANTLR imports so the core file stays stable.
grammar ShadowRust;
program: statement* EOF;
statement
    : ifStatement
    | txStatement
    | mintStatement
    | validateStatement
    | queueStatement
    | bankStatement
    | assignment
    ;
ifStatement: IF condition &apos;{&apos; statement* &apos;}&apos;;
txStatement: TX BUY ID FROM ID TO ID AMOUNT expr &apos;{&apos; statement* &apos;}&apos;;
mintStatement: MINT ID AMOUNT expr (EPOCH expr)? &apos;;&apos;;
validateStatement: VALIDATE ID STAGE NUMBER &apos;{&apos; statement* &apos;}&apos;;
queueStatement: QUEUE INSERT ID POSITIONS expr (&apos;,&apos; expr)* &apos;;&apos;;
bankStatement: BANK DEPOSIT ID ATR expr &apos;;&apos;;
assignment: ID &apos;=&apos; expr &apos;;&apos;;
condition: expr;
expr: relExpr (TO ID)?;
relExpr: arithExpr (op=(&apos;&gt;=&apos;|&apos;&gt;&apos;|&apos;&lt;=&apos;|&apos;&lt;&apos;|&apos;==&apos;|&apos;!=&apos;) arithExpr)*;
arithExpr: term (op=(&apos;+&apos;|&apos;-&apos;) term)*;
term: factor (op=(&apos;*&apos;|&apos;/&apos;) factor)*;
factor: ID | NUMBER | &apos;(&apos; expr &apos;)&apos;;
TX:&apos;tx&apos;; BUY:&apos;buy&apos;; FROM:&apos;from&apos;; TO:&apos;to&apos;; AMOUNT:&apos;amount&apos;;
IF:&apos;if&apos;; MINT:&apos;mint&apos;; EPOCH:&apos;epoch&apos;; VALIDATE:&apos;validate&apos;;
STAGE:&apos;stage&apos;; QUEUE:&apos;queue&apos;; INSERT:&apos;insert&apos;; POSITIONS:&apos;positions&apos;;
BANK:&apos;bank&apos;; DEPOSIT:&apos;deposit&apos;; ATR:&apos;atr&apos;;
ID: [a-zA-Z_][a-zA-Z0-9_]*;
NUMBER: [0-9]+ (&apos;.&apos; [0-9]+)?;
WS: [ \t\r\n]+ -&gt; skip;
COMMENT: &apos;//&apos; ~[\r\n]* -&gt; skip;
14.3 Required expansions before Feature-Complete Alpha
container { id=...; validators=...; hybrid=50; sync_tps=...; interval=...; }
network { listen=...; bootstrap=...; }
resilience if online &lt; 10 { activate sentinels; }
update_trait ID KEY op expr ;
vote PROPOSAL commitment ;
shard / async_stagger dialect imported from high_tps.g4
Keywords must precede the ID lexer rule. Dialects use ANTLR import. Regenerating the parser is a CI step.
14.4 Code-generation contracts
When the visitor sees tx buy ... it must emit Go that:
Builds a gnark circuit of the transfer statement.
Derives or reuses an ephemeral mirror for the session.
Routes any `expr TO address` as a fee commitment to that address (Vault by default).
Submits the proof into the mempool and tracks StageHints.
When it sees queue insert it must emit the unfair-position algorithm from section 5.4.1, under a mutex or with a single owner goroutine. When it sees bank deposit it must call the ATR math from section 11 and refuse to compile if the ATR operand is not bound to an oracle mock or client. When it sees validate ... stage N it must emit only the checks legal for that stage.
14.5 Example
Source:
tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05 to vault_address;
}
Must compile to a shielded transfer of 100 SFG-equivalent notes, a 5 SFG-equivalent fee commitment to the Vault, automatic ZKP, and no plaintext amount on the wire.
14.6 Compiler module layout
grammar/ShadowRust.g4 plus dialect files
parser/ generated lexer, parser, base visitor
ast/ Go structs
ir/ IRNode interface + transforms
sema/ type and security rules
interp/ mock environment
codegen/ Go emitter
plugins/ concurrency, privacy, high_tps visitors
Coverage gates: 80 percent at ShadowRust v0.1, 95 percent at v1.0.
15. High TPS Architecture
Year-1 public targets stay at 2,000–5,000 sustained TPS. The architecture below is mandatory so enterprise container load and later growth do not force an L2 that splits SFG liquidity.
15.1 Async staggered circuits
Multiple independent pipeline circuits run with time offsets (documents specify about 200 ms per step and 50 ms entry stagger). A transaction enters the next free circuit. Dynamic scaling starts extra circuits when mempool depth crosses a governance parameter. Each circuit still runs the five stages; staggering removes the “everyone waits on the same 1-second tick” cliff.
15.2 Sharding
16–64 shards. A note lives in one shard determined by a hash of its commitment. Cross-shard transfers use a ZK bridge inside L1: lock on source shard, prove inclusion, mint on destination shard. Containers pin to a shard or to a private subspace that later aggregates into one shard’s mega-batch.
15.3 Shielded L1 containers
A container is a mini-PC (8–16 cores, 32–64 GB RAM, 2–4 TB SSD is the enterprise planning envelope) that mirrors a business’s internal server. Rules:
Base 20 validators plus 2 per department.
Hybrid split: 50 percent of work is internal chained transactions, 50 percent of the business validators also join the public revolver.
Internal traffic runs the full five-stage pipeline with staggered circuits.
When internal TPS exceeds a threshold (example: 1,000) or an interval elapses (example: 5 minutes), the container aggregates a mega-batch and submits it as Kind ContainerSync.
Vault collects a sync fee. Native public validators who help verify syncs can receive a 20–50 percent bonus weight.
This is how the High TPS document claims multi-million theoretical TPS: most of a trillion-scale retail chain’s hops never touch the public revolver; only aggregates do.
15.4 Silent transactions and DoS
Sentinels and the Vault inject irregular (Poisson) null ZK transactions. They keep circuits from going cold, absorb burst junk, and give the monitor a baseline. If a wallet’s silent-adjusted rate spikes more than 20 percent above its baseline, the protocol can place a 7-day hold, take a 10 percent Vault fee, and open a burn-or-transfer vote. Business containers are whitelistable so payroll bursts are not treated as attacks. Appeals are staked TP votes.
15.5 Capacity sketch (design, not Year-1 promise)
Documents give a worked example: 64 shards × 50 circuits × about 9k TPS design each ≈ 28.8M TPS design capacity, while containers collapse a 5–11 trillion raw daily hop count into hundreds of millions of public transactions. Implementers must treat this as a capacity envelope to test toward, not as a launch SLA. Launch SLA remains the Success Metrics table in section 3.4.
16. Enterprise Adaptation
Goal: move a multi-department business off a 1–10 TB legacy server onto one or more L1 containers with zero planned downtime, 50–70 percent space reduction, and human sign-off at every phase. No autonomous AI rewrite.
16.1 Phase 1 — Assessment (weeks 1–2)
Read-only. rsync --dry-run, SQL dumps, log greps. Produce a JSON blueprint:
{ &quot;depts&quot;: [ { &quot;name&quot;: &quot;finance&quot;, &quot;traits&quot;: [&quot;income&quot;, &quot;balance&quot;] } ],
  &quot;workflows&quot;: [ { &quot;from&quot;: &quot;payment&quot;, &quot;to&quot;: &quot;tx update_trait finance balance += 500&quot; } ] }
Business signs the blueprint before anything is copied.
16.2 Phase 2 — Duplication (weeks 3–4)
Block-level snapshot (LVM/dd or VSS), ZFS/rsync differentials, isolated VM or mini-PC with matching IPs on a VLAN. Byte compare with cmp. Rollback is the snapshot.
16.3 Phase 3 — Synchronization (weeks 5–8)
This is the Parsing TXs/NFT flowchart:
Original or duplicate server emits inputs, logs, and state changes.
Human scripts plus ShadowRust parse (grep/SQL).
Parser generates a shielded transaction (trait update or transfer).
L1 container applies the five-stage pipeline and updates department NFT traits.
Shadow verification compares container outputs to the duplicate server. Mismatch blocks commit.
Bridge Proxy is a Rust or Go middleware that sits on the duplicate and forwards hooks without touching the live original.
16.4 Phase 4 — Testing (months 2–4)
Duplicate plus container on testnet. Department-by-department load (example 1,000 TX/day). Outage drill: pull the uplink, confirm internal mode, confirm megabatch sync after restore. Measure space (expect 30–50 percent of original on L1 plus decommissioning of replicas to reach 50–70 percent net), latency under 5 seconds, power 50–100 W. External audit before any mainnet shadow.
16.5 Phase 5 — Cutover (months 5–6)
30-day mainnet shadow. Governance vote. Flip the bridge flag so all traffic hits the container. Decommission the original only after the business signs the migration-complete report. Post-cutover dashboards live in the Creation App.
16.6 Mapping rules
Department → soulbound NFT with typed traits.
Workflow step → ShadowRust statement, usually update_trait or tx buy.
Cross-department posting → shielded transfer between two department notes.
Payroll bursts → container-local batch + whitelist so silent-TX DoS logic does not freeze the firm.
17. End-to-End Flows
17.1 Ordinary shielded payment
Alice’s wallet, session already holding a live mirror, builds a transfer circuit: spend Alice notes, create Bob note, fee to Vault.
Wallet may briefly drop the network while the key is used, then reconnect and submit.
Mempool admits the blob. Next 1-second batcher picks it up.
Revolver pops validators for stages 1–5 (or 2–3 per stage if the network is large).
Stage 1 reserves Alice’s nullifier. Stage 2 checks form. Stage 3 accepts Bob’s note parameters. Stage 4 writes a candidate Merkle root. Stage 5 collects BFT votes and commits.
Bob’s wallet scans commitments with his viewing material and shows a new shielded balance. Explorers show only a hash.
17.2 Free NFT mint
Wallet requests micro-drop. Anti-bot oracle + CAPTCHA pass.
Mint transaction of Kind Mint, amount 1 NFT, one-per-wallet constraint inside the circuit / contract.
Five stages. Trait map starts empty.
App session token issued. User may toggle Validate, which schedules revolver insert at the unfair positions.
17.3 Bank deposit then later withdraw
Follow section 11 exactly. The on-chain objects are BankDeposit and BankWithdraw shielded transactions plus a BankHold record visible to the Bank operator and to the user via reveal key, not to the public explorer.
17.4 Epoch mint
Holder with Mint toggled submits a mint proposal after epoch start.
Votes accumulate as ZKP ballots during the epoch.
At epoch end, if votes pass the governance threshold, SFG is minted into the proposer path chosen in the App (direct with 10 percent fee, or staked 2 percent yield path).
Vault pays epoch bonuses from the 20 percent slice, applying the +20 percent / −10 percent toggle rule.
17.5 Outage then recovery
Follow section 5.6. Wallets keep an outbound queue. Containers switch to internal mode. Sentinels fill the revolver if needed. Recovery is megabatch plus dual track.
17.6 Enterprise trait update
Follow section 16.3. Commit is illegal if shadow verification fails.
18. Implementation Blueprint
This section turns Development Plan V2 and Phase 0 nano-steps into a build order. Document dates assumed a Q1 2026 start. This master spec is compiled 27 August 2026; keep the sequence, shift calendars as the team actually starts coding.
18.1 Repository layout
shadowforge-l1/
  cmd/node/          // node entrypoint
  cmd/hello/          // Phase 0 sanity binary
  cmd/shadowc/        // ShadowRust CLI (parse, lint, interp, gen)
  grammar/            // ShadowRust.g4 + dialects
  parser/             // ANTLR generated Go
  ast/ ir/ sema/ interp/ codegen/ plugins/
  pkg/net/            // libp2p + Noise + heartbeat
  pkg/consensus/      // epoch, revolver, BFT, sentinel, megabatch
  pkg/state/          // Badger + Merkle + encryption
  pkg/tx/             // mempool, 5-stage pipeline, kinds
  pkg/zk/             // gnark circuits
  pkg/crypto/         // Dilithium wrappers
  pkg/bank/           // ATR math, holds, oracle client
  pkg/vault/          // fee sinks, allocation
  pkg/nft/            // mint, traits, soulbind
  pkg/container/      // enterprise subspace + sync
  mocks/              // in-memory state for tests
  deployments/docker/ // multi-node compose
  docs/               // this specification
wallet/               // separate Flutter+Rust repo or /wallet
app/                  // Creation App Flutter + ShadeLang JS
Branches: main (stable), dev, feature/*. CI: go test ./..., golangci-lint, antlr generation check, Docker multi-node smoke.
18.2 Phase 0 — Toolchain (marked complete in V2, still the bootstrap if a new machine is imaged)
Install Go 1.21+ (plans mention 1.23.x).
Install JDK + ANTLR complete JAR 4.13.x, alias antlr4, go install the ANTLR Go runtime.
go install gnark.
Dilithium community module (kudelskisecurity or pqcrypto equivalent). Pin a version in go.mod.
Docker, Git, golangci-lint.
Repo with go.mod, README, mocks/state.go, cmd/hello, passing go test ./... .
Verification: hello prints, tests green, CI green.
18.3 Phase 1 — ShadowRust MVP (3–5 weeks)
Commit the grammar file exactly, then generate lexer/parser/visitor.
AST structs for every statement kind.
Interpreter for expr, assignment, and a mocked tx that does not yet prove.
Codegen for tx → Go stub that later grows a real gnark circuit.
Parse the sample tx buy ... and the queue insert snippet. No crashes. 80 percent coverage.
Internal review week: run whitepaper queue-insert pseudocode through the toolchain. Update this spec if the grammar must change.
18.4 Phase 2 — L1 core (3–4 months)
State module with encrypted Badger and Merkle. Fuzz.
Consensus: epoch clock, revolver with unfair insert, adaptive 1/2/3 validators per stage, TLA+ sketch of BFT + queue invariants, sentinel flag, outage path.
Networking: bootstrap, heartbeats, Docker four-node net.
TX processing: mempool, 1-second batch, five stages, gnark verify, Kind enum.
Checkpoint: local net stable at 1,000 TX/min. That is an engineering checkpoint, not the public Success Metrics bar.
18.5 Phase 3 — Features (4–5 months)
Dilithium everywhere, reveal keys, KYC oracle hook.
Wallet prototype (section 12 nano-plan).
NFT mint, TP, toggle, lottery giveaway contract.
Bank with the exact ATR math and safeguards.
Creation App MVP + Blockly/Monaco talking to shadowc.
Green oracle bonus hook.
Migration tools: parser scripts, shadow-verify, container skeleton.
ShadowRust v1.0, 95 percent coverage, IR transforms.
Checkpoint: feature-complete alpha, load test against the Success Metrics Year-1 TPS bar, stretch toward the High TPS harness.
18.6 Phase 4 — Hardening (2–3 months)
90 percent unit/integration coverage, CI gate.
Go-fuzz on pipeline and Bank math. 24-hour fuzz runs.
TLA+ or equivalent on revolver + megabatch.
External audit 4-week window. Fix all severes.
Sharding and staggered circuits turned on in staging.
Checkpoint: public testnet, 99 percent uptime over the trial window.
18.7 Phase 5 — Launch
Testnet on Kubernetes.
Genesis ceremony, first NFT giveaway, Vault keys in multisig.
Mainnet. Then bridges, business stablecoin path, more containers.
18.8 Daily engineering rules from V2
4–6 hours of focused coding on the current nano-step.
Commit only with tests.
Track work in issues / Kanban matching these phases.
Budget on the order of $50k for audits (Vault 10 percent slice later replaces this).
19. Algorithms and Formulas (copy into tests)
19.1 Epoch duration
func EpochDuration(n uint64) time.Duration {
    // 1 hour * 1.1^n, cap 365d
    hours := math.Pow(1.1, float64(n)) // use big.Rat in production
    d := time.Duration(hours * float64(time.Hour))
    year := 365 * 24 * time.Hour
    if d &gt; year { return year }
    return d
}
19.2 Unfair revolver insert
func InsertValidator(q *deque.Deque[ID], addr ID) {
    positions := []int{ q.Len(), 4, 10, 2, 7 }
    for _, pos := range positions {
        if q.Contains(addr) { return }
        idx := pos % (q.Len() + 1)
        q.Insert(idx, addr)
    }
}
19.3 Bank deposit
buffer    := Decimal(2.5).Mul(currentATR_USD)
net       := grossUSD.Sub(buffer)
if net.Sign() &lt;= 0 { reject }
fee       := net.Mul(Decimal(&quot;0.001&quot;))
sfgIssued := net.Sub(fee).Div(sfgUSD)
19.4 Bank withdraw
retention := Decimal(1.5).Mul(avgATR_USD)
refund    := max(0, entryBuffer.Sub(retention))
// repaySFG includes asymmetry + slippage(0.002..0.005) + 0.001 exit + gas
19.5 Adaptive stage width
func ValidatorsPerStage(online int) int {
    switch {
    case online &gt; 300: return 3
    case online &gt; 200: return 2
    default:           return 1
    }
}
19.6 Sentinel trigger
if revolver.Online() &lt; 10 { activateSentinels(10) }
19.7 Multi-task bonus
mult := 1.0
if validateOn &amp;&amp; mintOn { mult = 1.20 }
if xor(validateOn, mintOn) { mult = 0.90 } // or badge instead of cash
payout := baseBonus.Mul(mult).Mul(tpWeight)
20. Testing Matrix
Layer
Must pass
Grammar
Every sample in this spec parses. Invalid tx without amount fails sema.
Interpreter
Arithmetic, fee routing TO vault, mocked bank deposit math.
Pipeline
Happy path 5 stages; fail at each stage rolls back nullifier; concurrent 1s batches.
Revolver
Unfair insert positions; duplicate ignored; cooldown exclusion; &lt;10 activates sentinels.
Epoch
Sum of durations matches wall clock across 0..N including the year cap.
ZK
Valid note spends verify; double-spend nullifier rejected; amount mismatch rejected.
Bank
Buffer reject when ATR too high; refund never negative; 24h lock; 4th cycle surcharge.
Net
Four Docker nodes finalize a batch; peer flood is rate-limited.
Outage
Kill 60 percent of nodes; backlog drains via megabatch dual-track.
Container
Shadow verify mismatch blocks trait commit; internal mode on uplink loss.
Wallet
Mirror unused after session; offline disconnect invoked; hardware mode never sees raw key in UI process.
21. Monitoring
Public explorer: shielded views only (counts, roots, epoch, queue length, sentinel flag). Internal dashboard (Creation App + Prometheus): TPS, latency histogram, revolver size, TP distribution, Bank hold count, oracle disagreement, silent-TX fraction, container sync lag, Vault balances. If a Success Metrics category misses by more than 20 percent, open a governance upgrade proposal rather than silently retuning economics.
22. Governance Parameters (genesis defaults)
Parameter
Default
Who may change
Batch interval
1 second
Governance
Stage timeout
4 seconds
Governance
Heartbeat
10 seconds, offline after 3 misses
Governance
Cooldown
1 hour
Governance
Sentinel threshold
10 online
Governance
Adaptive widths
2 at &gt;200, 3 at &gt;300
Governance
Deposit ATR multiple
2.5
Governance
Withdraw ATR multiple
1.5
Governance
Bank fees
0.1% in and out
Governance
Bank daily cap
$100,000
Governance + KYC raise
Cycle surcharge
&gt;3 / 30d → +1%
Governance
Deposit lock
24 hours
Governance
Slippage range
0.2–0.5%
Oracle + governance bounds
Inflation
2–5% / year
Governance
Vault splits
20 / 10 / 10 / 60
Governance
KYC prompt threshold
$10,000 example
Governance
Silent-TX spike hold
+20%, 7 days, 10% fee
Governance
Container hybrid split
50 / 50
Per-container, governance envelope
23. Risks That Remain
gnark / circuit bugs. Mitigation: tiny circuits, recursive later, external audit, fuzz of the prover/verifier pair.
Oracle manipulation of ATR. Mitigation: quorum, freeze-on-disagreement, governance.
NFT sybils through weak PoH. Mitigation: one-per-wallet plus TP delay plus optional Worldcoin.
Container operator censors internal traffic. Mitigation: 50 percent native validators on sync path, DA roots public.
Wallet malware. Mitigation: virtual state, offline windows, hardware mode, mouse-score as a weak human signal only.
Legal pressure on privacy. Mitigation: reveal keys and optional KYC, not a kill-switch.
Year-1 TPS disappointment versus High TPS marketing. Mitigation: this document separates SLA from architecture. Do not advertise 1M TPS as a launch guarantee.
24. Glossary
Term
Meaning
ATR
Average True Range. Volatility input to Bank buffers, in USD.
BankHold
Open record of an external-asset deposit waiting to be closed.
Container
Enterprise mini-PC running a shielded L1 subspace.
Creation App
NFT-gated Flutter hub for mint, vote, ShadeLang, Bank.
DARoot
Merkle root of data-availability blobs in a block.
Dilithium
Post-quantum signature scheme used on stages and wallets.
Dual-track
Live track plus backlog track during megabatch recovery.
Epoch
Slow clock: 1 hour × 1.1^n, capped at one year.
Ephemeral mirror
Session-only address burned after use.
gnark
Go ZK library used for SNARK circuits.
Megabatch
10× batch used to drain outage backlog.
Note
Private unspent value object; commitment is public.
Nullifier
Public spend tag that prevents double-spend.
PoA
Proof of Authority — NFT + liveness, not coin weight.
Reveal key
User secret that opens chosen transactions for audit.
Revolver
Deque of live validators with unfair join inserts.
Sentinel
Protocol node that appears when online validators &lt; 10.
SFG
Native token. 500M genesis. Fees in SFG.
ShadeLang
Visual/low-code frontend that emits ShadowRust.
ShadowRust
ANTLR DSL compiled to Go for L1 logic.
ShadowVault
Fee treasury with fixed default splits.
Silent TX
Null ZK pad used for warmth, desync, and DoS absorption.
Soulbound
NFT that cannot be transferred until governance unlocks it.
TP
Trust Points from uptime and honest stage work.
Five stages
Sender Leave, TX Offer, Receiver Check, Send Exec, Place Final.
25. Source Map
Every requirement in this master spec traces to the project corpus. If a future chat changes a number, update this table and the numbered section together.
Topic
Authoritative source
Vision, epochs, five stages, revolver idea, Vault story
Whitepaper.txt
Bank 2.5× / 1.5× correction, STRIDE, SFG-only L1
Spec V1.0
Wallet as a first-class module in the unified layout
Layout V1.0
Circuits, shards, containers, silent TX, DoS holds
High TPS Specification
Wallet fortress features and nano-plan
Wallet Design
Migration five phases, dept NFTs, space savings
Enterprise Adaption Plan
Parse → generate TX → trait update → shadow verify
Parsing TXs_NFT.pdf
Linear outage pipeline
Outage Handling.pdf
Migration swimlane
Migration Process Flowchart.pdf
Grammar tokens and rules
shadowrust.g4.txt
Compiler components and nano-steps
Overall Structure of ShadowRust.txt + Backend Lang.txt
Why ShadeLang looks like Blockly + Monaco + sandbox
ShadowLang research.txt
Phase order and checkpoints
Development Plan V2 + Phase 0 Nano steps
Public numeric bars
Success Metrics
ATR narrative plus adaptive queue and sentinels
Detailed Breakdown
26. What to Build First Tomorrow
If the toolchain from Phase 0 is already on the machine, the next concrete commit is:
Place shadowrust.g4 in grammar/ and generate the Go parser in CI.
Implement ast + a visitor that prints the tree for the sample tx buy program.
Implement expr evaluation in the interpreter with tests for 100 * 0.05.
Implement InsertValidator with table-driven tests for the five positions.
Implement EpochDuration with tests at n=0, n=10, and the year cap.
Do not start Flutter, Bank oracles, or sharding until those five artifacts are green.
That is the entire currently specified system: a private-by-default, NFT-authorized, epoch-paced Layer 1, with a volatility firewall at the edge, a visual language on top, an enterprise container path on the side, and a Go compiler in the middle. Anything not named in this document is out of scope for v1 and needs a governance or spec revision before it is coded.
