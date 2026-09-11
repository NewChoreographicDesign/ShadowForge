// Command mintpage serves ShadowForge's real, minimal validator-NFT
// minting page: a small web frontend plus the Go backend it needs, since
// unlike cmd/explorer's read-only pages, minting has to build and sign a
// real TxNFTMint (pkg/txbuilder.Builder.NFTMint) and submit it over a
// real libp2p connection (pkg/txclient) — work a browser alone can't do,
// since this codebase's post-quantum Dilithium signing only exists in Go
// (pkg/crypto), not as a JS library. This binary is a thin HTTP wrapper
// around exactly the same real packages cmd/wallet's "identity",
// "poh-attest", and "nft-mint" subcommands already use — same keystore
// format (pkg/walletkey), same attestation (pkg/nft), same builder
// (pkg/txbuilder), same submit-and-confirm path (pkg/txclient). Nothing
// here is a second, parallel implementation.
//
// The real spec-10.1 CAPTCHA/proof-of-humanity challenge itself is still
// explicitly out of this L1 core's scope (see pkg/nft's own doc): this
// page's "Attestor" panel does not decide whether anyone is human, it
// only signs the real, verifiable claim after whoever runs this page has
// decided that by their own real means (spec 10.1's intent is a service
// doing that at scale; for a small testnet, that can just be the operator
// looking at who's asking).
//
// Every wallet passphrase submitted to this page passes through this
// server in plaintext (the same trust boundary as typing it into a local
// CLI prompt) — see -listen's own doc for why this binds to loopback by
// default.
package main

import (
	"context"
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed static/app.js static/style.css
var staticFS embed.FS

//go:embed static/index.html
var indexHTML string

// maxUploadBytes bounds any single request body this server reads (a
// keystore file is a few KB) — a generous ceiling against abuse, not a
// real expected size.
const maxUploadBytes = 1 << 20

type server struct {
	maxConfirmTimeout time.Duration
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8096", "address to serve the mint page on. Loopback by default: every request here can carry a real wallet passphrase in plaintext (the same trust boundary as typing one into a local CLI prompt) and this binary does no TLS itself — binding beyond loopback is this operator's explicit choice, and should sit behind a real TLS-terminating reverse proxy")
	defaultBootstrap := flag.String("default-bootstrap", "", "default bootstrap peer multiaddr to prefill in the page (a visitor can still type a different one)")
	defaultBootstrapFile := flag.String("default-bootstrap-file", "", "path to wait for and read a default bootstrap multiaddr from at startup (pairs with a validator's own -announce-file over a shared volume, e.g. Docker Compose) — takes precedence over -default-bootstrap if both are set")
	defaultQuery := flag.String("default-query", "http://127.0.0.1:8081", "default pkg/query base URL to prefill in the page (a visitor can still type a different one)")
	maxConfirmTimeout := flag.Duration("max-confirm-timeout", 60*time.Second, "hard ceiling on how long a single mint request will keep an HTTP request open waiting for confirmation, regardless of what the page asks for")
	flag.Parse()

	if *defaultBootstrapFile != "" {
		addr, err := waitForAddrFile(context.Background(), *defaultBootstrapFile)
		if err != nil {
			log.Fatalf("mintpage: -default-bootstrap-file: %v", err)
		}
		*defaultBootstrap = addr
	}

	tmpl, err := template.New("index.html").Parse(indexHTML)
	if err != nil {
		log.Fatalf("mintpage: parse index.html: %v", err)
	}
	assets, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("mintpage: sub fs: %v", err)
	}
	fileServer := http.FileServerFS(assets)

	s := &server{maxConfirmTimeout: *maxConfirmTimeout}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := struct {
			DefaultBootstrap string
			DefaultQuery     string
		}{*defaultBootstrap, *defaultQuery}
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("mintpage: render index: %v", err)
		}
	})
	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		fileServer.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fileServer.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /api/generate", s.handleGenerate)
	mux.HandleFunc("POST /api/identity", s.handleIdentity)
	mux.HandleFunc("POST /api/attest", s.handleAttest)
	mux.HandleFunc("POST /api/mint", s.handleMint)

	if !strings.HasPrefix(*listen, "127.0.0.1") && !strings.HasPrefix(*listen, "localhost") && !strings.HasPrefix(*listen, "[::1]") {
		log.Printf("WARNING: -listen=%s is not loopback-only — every request to this page can carry a real wallet passphrase in plaintext and this binary does no TLS itself; only do this behind your own TLS-terminating reverse proxy", *listen)
	}

	srv := &http.Server{Addr: *listen, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("mintpage: serve: %v", err)
		}
	}()
	log.Printf("mintpage: serving on %s", *listen)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("mintpage: shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
