// Command statuspage is ShadowForge's real, minimal public status page
// (Phase 3's "hosting, monitoring, log aggregation, status page"
// roadmap item): unlike cmd/explorer, which reads live chain state
// straight from a browser (pkg/query's own doc explains why that's safe
// and sufficient there), uptime history has to be observed continuously
// by something that keeps running whether or not anyone has the page
// open — so this binary itself polls every configured node's real
// /v1/status on an interval (pkg/statuscheck), keeps a real, capped,
// optionally-persisted history, and serves a small page rendering it.
//
// Nothing here is simulated: a node that's actually down is recorded as
// down (a real connection/timeout error, not swallowed), and the
// "uptime percent" and history strip shown are computed directly from
// those real recorded checks.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/statuscheck"
)

//go:embed static/app.js static/style.css
var staticFS embed.FS

//go:embed static/index.html
var indexHTML string

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "address to serve the status page on")
	nodesFlag := flag.String("nodes", "", "comma-separated name=queryBaseURL pairs to monitor, e.g. validator1=http://127.0.0.1:8081,validator2=http://127.0.0.1:8082 (required)")
	dataPath := flag.String("data", "", "path to persist check history across restarts (JSON file) — empty means in-memory only, history is lost on restart")
	interval := flag.Duration("interval", 30*time.Second, "how often to poll every configured node's real /v1/status")
	timeout := flag.Duration("timeout", 5*time.Second, "per-node HTTP timeout for each check")
	history := flag.Int("history", 500, "how many recent checks to retain per node")
	flag.Parse()

	nodes, err := parseNodes(*nodesFlag)
	if err != nil {
		log.Fatalf("statuspage: -nodes: %v", err)
	}

	mon, err := statuscheck.NewMonitor(statuscheck.Config{
		Nodes:       nodes,
		MaxHistory:  *history,
		PersistPath: *dataPath,
		Logf:        log.Printf,
	})
	if err != nil {
		log.Fatalf("statuspage: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mon.Run(ctx, *interval, *timeout)

	tmpl, err := template.New("index.html").Parse(indexHTML)
	if err != nil {
		log.Fatalf("statuspage: parse index.html: %v", err)
	}

	assetsSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("statuspage: sub fs: %v", err)
	}
	fileServer := http.FileServerFS(assetsSub)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, struct{ RefreshSeconds int }{int((*interval).Seconds())}); err != nil {
			log.Printf("statuspage: render index: %v", err)
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
	mux.HandleFunc("GET /api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(mon.Snapshot()); err != nil {
			log.Printf("statuspage: encode snapshot: %v", err)
		}
	})

	srv := &http.Server{Addr: *listen, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("statuspage: serve: %v", err)
		}
	}()
	log.Printf("statuspage: serving on %s, monitoring %d node(s), polling every %s", *listen, len(nodes), *interval)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("statuspage: shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// parseNodes decodes -nodes's comma-separated name=queryBaseURL pairs.
func parseNodes(raw string) ([]statuscheck.NodeConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("at least one name=queryBaseURL pair is required")
	}
	var out []statuscheck.NodeConfig
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 || eq == len(part)-1 {
			return nil, fmt.Errorf("malformed entry %q, expected name=queryBaseURL", part)
		}
		out = append(out, statuscheck.NodeConfig{
			Name:      strings.TrimSpace(part[:eq]),
			QueryBase: strings.TrimSpace(part[eq+1:]),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one name=queryBaseURL pair is required")
	}
	return out, nil
}
