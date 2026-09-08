// Command explorer serves ShadowForge's real, minimal public block
// explorer (Phase 3's "public testnet" roadmap item): a static,
// dependency-free frontend that talks directly to a live validator's
// pkg/query HTTP API from the browser — pkg/query's own CORS middleware
// already allows this (see that package's doc) — so there is no
// server-side proxy, no separate database, and no second copy of chain
// state to keep in sync. Every figure the page shows is exactly what
// pkg/query already answers any stranger over HTTP; this binary's only
// job is to serve the static frontend and tell it, by default, which
// node to talk to.
//
// -query sets that default only: a viewer can still point their own
// browser at a different node entirely by editing the "Node" field the
// page itself exposes (persisted in that browser's own localStorage),
// since nothing here is hardcoded past the first paint.
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed static/app.js static/style.css
var staticFS embed.FS

//go:embed static/index.html
var indexHTML string

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "address to serve the explorer on")
	queryBase := flag.String("query", "http://127.0.0.1:8081", "default pkg/query API base URL the explorer points at (a viewer can still override this from the page itself)")
	flag.Parse()

	tmpl, err := template.New("index.html").Parse(indexHTML)
	if err != nil {
		log.Fatalf("explorer: parse index.html: %v", err)
	}

	assets, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("explorer: sub fs: %v", err)
	}
	fileServer := http.FileServerFS(assets)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, struct{ DefaultQueryBase string }{*queryBase}); err != nil {
			log.Printf("explorer: render index: %v", err)
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

	fmt.Fprintf(os.Stderr, "explorer: serving on %s (default query API: %s)\n", *listen, *queryBase)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatalf("explorer: %v", err)
	}
}
