package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError reports a request-side or server-side failure as a JSON
// body rather than a bare HTTP status — every fetch() call in app.js reads
// {"error": "..."} uniformly regardless of status code, so the page can
// show one real, specific message instead of a generic "something went
// wrong".
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// saveUploadedFile copies a multipart file field to a private temp file
// and returns its path plus a cleanup func — walletkey.Load needs a real
// path, not an io.Reader, so every handler that accepts an uploaded
// keystore goes through this rather than parsing the format itself.
// cleanup always removes the temp file; callers must defer it.
func saveUploadedFile(r *http.Request, field string) (path string, cleanup func(), err error) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return "", func() {}, fmt.Errorf("parse form: %w", err)
	}
	file, _, err := r.FormFile(field)
	if err != nil {
		return "", func() {}, fmt.Errorf("missing file field %q: %w", field, err)
	}
	defer file.Close()

	f, err := os.CreateTemp("", "mintpage-upload-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	cleanup = func() { _ = os.Remove(tmpPath) }

	if _, err := io.Copy(f, io.LimitReader(file, maxUploadBytes)); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("save upload: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("save upload: %w", err)
	}
	return tmpPath, cleanup, nil
}

// waitForAddrFile polls for path to appear and returns its trimmed
// contents — mirrors cmd/wallet/network.go's identical helper of the
// same purpose (kept duplicated rather than shared, the same per-binary
// convention that helper's own doc explains).
func waitForAddrFile(ctx context.Context, path string) (string, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b)), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for %s", path)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
