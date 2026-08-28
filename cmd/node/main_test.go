package main

import (
	"path/filepath"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// TestLoadOrCreateStateKeyPersistsAcrossRestarts is a direct regression
// test for a real bug live auditing surfaced: -data <dir> previously
// generated a fresh random encryption key on every process start,
// meaning a real node could never actually reopen its own persisted
// data after any restart — a randomly different key each time can't
// decrypt notes sealed under the previous run's key. This proves the
// fix: the same dataDir yields the same real key across independent
// calls (simulating separate process starts), and a note encrypted and
// stored in one "run" decrypts correctly when a fresh *state.Store opens
// the same directory with the key loadOrCreateStateKey reloads.
func TestLoadOrCreateStateKeyPersistsAcrossRestarts(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nodedata")

	firstRunKey, err := loadOrCreateStateKey(dataDir)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	secondRunKey, err := loadOrCreateStateKey(dataDir)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if firstRunKey != secondRunKey {
		t.Fatalf("expected the same dataDir to yield the identical real key across restarts, got two different keys")
	}

	// A real end-to-end proof, not just equal key bytes: open a real
	// Store, write a real encrypted note, close it (simulating a
	// process exit), then open a *fresh* Store on the same directory
	// with a key reloaded exactly as a real restart would — the note
	// must still decrypt correctly.
	store1, err := state.Open(dataDir, false, firstRunKey)
	if err != nil {
		t.Fatalf("open store (first run): %v", err)
	}
	note := types.Note{Commitment: types.Hash{0x42}, Value: 777, OwnerPk: []byte("owner"), Rho: []byte("rho"), Asset: "SFG"}
	if err := store1.PutNote(note); err != nil {
		t.Fatalf("put note: %v", err)
	}
	if err := store1.Close(); err != nil {
		t.Fatalf("close store (first run): %v", err)
	}

	restartKey, err := loadOrCreateStateKey(dataDir)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	store2, err := state.Open(dataDir, false, restartKey)
	if err != nil {
		t.Fatalf("open store (restart): %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })

	got, found, err := store2.GetNote(note.Commitment)
	if err != nil {
		t.Fatalf("get note after restart: %v", err)
	}
	if !found {
		t.Fatalf("expected the note written before restart to still be found after restart")
	}
	if got.Value != note.Value {
		t.Fatalf("expected the note to decrypt to its real value 777 after restart, got %d", got.Value)
	}
}

// TestLoadOrCreateStateKeyEphemeralWhenNoDataDir confirms in-memory mode
// (no -data) keeps its prior, correct behavior: a fresh random key every
// call, since there is no on-disk data a mismatched key could ever break.
func TestLoadOrCreateStateKeyEphemeralWhenNoDataDir(t *testing.T) {
	k1, err := loadOrCreateStateKey("")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	k2, err := loadOrCreateStateKey("")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if k1 == k2 {
		t.Fatalf("expected two ephemeral (dataDir-less) keys to differ")
	}
}
