// Package query is a real, read-only HTTP JSON API over a validator
// node's live chain state — Tier B's foundation from the Ignition/Horizon
// roadmap: nothing else a wallet needs (balance-equivalent lookups,
// transaction status, governance tallies) is honestly buildable without a
// way to ask a node what it actually knows.
//
// Every endpoint answers from the same live *state.Store, *chain.Chain,
// and *tx.Mempool a running validator.Node already uses — there is no
// separate, potentially-stale read replica, and nothing here is mocked or
// hard-coded. What this package deliberately does NOT expose:
//
//   - Shielded note contents (value, owner key, nullifier seed). Notes
//     are looked up by commitment for existence only — GetNote decrypts
//     real private fields for the pipeline's own use, and this package
//     discards everything but the found/not-found bit before a response
//     is ever constructed, so a public query can never become a way to
//     read a shielded note's plaintext value or owner.
//   - Per-voter governance data. state.ProposalRecord's Commitments/
//     Reveals maps name which NFTID cast which ballot; the proposal
//     endpoints return only the aggregate tally (counts and pass/fail),
//     via an explicit response type that has no field to carry them.
//
// Nothing here requires authentication — every value returned already
// either mirrors data real consensus already gossips to every peer
// (blocks, transactions, NFTs, holds) or is a boolean existence/status
// check that reveals nothing beyond "yes/no" (nullifier spent, note
// exists, tx status). A per-IP rate limiter (ratelimit.go) is the only
// real defense this needs, since it is meant to be reachable by strangers
// once a node operator publishes it.
package query

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Server is a read-only HTTP JSON query server over one validator node's
// live state. Every field is a real, shared reference to the same
// objects cmd/node's validator.Node uses — this server never owns or
// copies chain state itself.
type Server struct {
	store   *state.Store
	chn     *chain.Chain
	mempool *tx.Mempool
	genesis int64

	logf func(format string, args ...any)

	httpSrv *http.Server
	limiter *ipRateLimiter

	addrMu sync.Mutex
	addr   string
}

// Config configures a Server.
type Config struct {
	// ListenAddr is the address to bind, e.g. "127.0.0.1:8081". Binding
	// to a loopback address is the safe default for a node an operator
	// hasn't deliberately decided to expose; binding to a non-loopback
	// address is the caller's explicit choice to publish it.
	ListenAddr string
	// GenesisMs is the chain's genesis time, unix milliseconds — included
	// in /v1/status purely so a client can independently compute epoch
	// timing without needing its own copy of that config out-of-band.
	GenesisMs int64
	// Logf receives operational log lines (server start/stop, request
	// errors worth surfacing). Defaults to log.Printf if nil.
	Logf func(format string, args ...any)
}

// NewServer builds a query Server backed by the given live store, chain,
// and mempool. It does not start listening — call Start.
func NewServer(store *state.Store, chn *chain.Chain, mempool *tx.Mempool, cfg Config) *Server {
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	s := &Server{
		store:   store,
		chn:     chn,
		mempool: mempool,
		genesis: cfg.GenesisMs,
		logf:    logf,
		limiter: newIPRateLimiter(defaultRateLimit, defaultBurst),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/blocks/{height}", s.handleBlock)
	mux.HandleFunc("GET /v1/tx/{txid}", s.handleTx)
	mux.HandleFunc("GET /v1/nullifier/{hash}", s.handleNullifier)
	mux.HandleFunc("GET /v1/note/{commitment}", s.handleNote)
	mux.HandleFunc("GET /v1/nft/{id}", s.handleNFT)
	mux.HandleFunc("GET /v1/hold/{id}", s.handleHold)
	mux.HandleFunc("GET /v1/proposal/{id}", s.handleProposal)
	mux.HandleFunc("GET /v1/proposals", s.handleProposals)

	var handler http.Handler = mux
	handler = s.limiter.middleware(handler)
	handler = corsMiddleware(handler)
	handler = s.loggingMiddleware(handler)

	s.httpSrv = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	return s
}

// Start binds the listener and serves until ctx is cancelled, at which
// point it shuts the HTTP server down gracefully. It blocks until the
// listener is confirmed bound (so a caller logging "ready" afterward is
// telling the truth) and returns any bind error immediately; the actual
// serve loop and shutdown run in background goroutines.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("query: listen on %s: %w", s.httpSrv.Addr, err)
	}

	s.addrMu.Lock()
	s.addr = ln.Addr().String()
	s.addrMu.Unlock()

	go s.limiter.cleanupLoop(ctx.Done())

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logf("query: serve error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			s.logf("query: shutdown error: %v", err)
		}
	}()

	s.logf("query: listening on %s", ln.Addr())
	return nil
}

// Addr returns the actual bound address (host:port) once Start has
// succeeded — useful when ListenAddr's port was 0 (let the OS choose) and
// a caller needs to know which port was actually picked. Empty before
// Start is called.
func (s *Server) Addr() string {
	s.addrMu.Lock()
	defer s.addrMu.Unlock()
	return s.addr
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		s.logf("query: %s %s from %s", r.Method, r.URL.Path, remoteIP(r))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseHash decodes a 64-hex-character path parameter into a types.Hash,
// rejecting anything the wrong length or not valid hex rather than
// silently truncating or zero-padding it into a different value.
func parseHash(s string) (types.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.Hash{}, fmt.Errorf("not valid hex: %w", err)
	}
	if len(b) != len(types.Hash{}) {
		return types.Hash{}, fmt.Errorf("expected %d bytes, got %d", len(types.Hash{}), len(b))
	}
	var h types.Hash
	copy(h[:], b)
	return h, nil
}

// parseNFTID decodes a 64-hex-character path parameter into a types.NFTID.
func parseNFTID(s string) (types.NFTID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.NFTID{}, fmt.Errorf("not valid hex: %w", err)
	}
	if len(b) != len(types.NFTID{}) {
		return types.NFTID{}, fmt.Errorf("expected %d bytes, got %d", len(types.NFTID{}), len(b))
	}
	var id types.NFTID
	copy(id[:], b)
	return id, nil
}
