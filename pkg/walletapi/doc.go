// Package walletapi is the one stable function surface the ShadowForge
// wallet app binds on every platform — Dart FFI (via a c-shared/
// c-archive build) on iOS and Android, GOOS=js GOARCH=wasm on the web.
// It never touches a UI toolkit and holds no long-lived state of its
// own; every exported function is a thin, synchronous wrapper around
// this repo's own real, already-tested packages (pkg/walletkey,
// pkg/shieldedwallet, pkg/queryclient, pkg/txclient, pkg/net) — nothing
// here reimplements Dilithium, X25519, or Groth16 proving, and nothing
// here ever lets a private key leave the calling process. See the
// ShadowForge Wallet Blueprint (the L1 roadmap's own §4) for the full
// architecture this package is the first slice of.
//
// Every exported type is deliberately flat — primitive fields only, no
// nested interfaces or generics — because the real binding boundaries
// this package is built to cross (a C ABI via Dart FFI, and Go/Wasm's
// own syscall/js bridge) only carry that shape cleanly.
//
// Deliberately NOT exposed here: the *-zk-setup family (pkg/zk.Setup and
// its siblings). Those generate the shared Groth16 params files a whole
// network agrees on; an end-user wallet only ever consumes one that
// already exists, the same way 'wallet transfer -zk-params <file>' takes
// a path rather than generating one inline — see loadZKSystem's own doc.
//
// Milestone-1 scope only, for now: identity, balance, and real shielded
// transfer. PoH/NFT-mint, governance, bank, and staking follow the same
// pattern (see cmd/wallet's own subcommands, each a real reference
// implementation of the flow to wrap next).
package walletapi
