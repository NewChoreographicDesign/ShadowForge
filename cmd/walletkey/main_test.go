package main

import (
	"strings"
	"testing"
)

// TestReadPassphraseLineReadsSequentialLines is a direct regression test
// for a real bug found via live CLI testing: an earlier version created a
// fresh bufio.Scanner on every call, which silently lost whatever a prior
// call's scanner had already buffered past the first newline — so
// `walletkey passwd -passphrase-stdin`, which needs an old passphrase
// line followed by a new one, would read the old line fine and then fail
// on the new one with "no passphrase line found on stdin" even though it
// was sitting right there in the pipe. This pins the fix (a single,
// lazily-created, shared scanner) in place.
func TestReadPassphraseLineReadsSequentialLines(t *testing.T) {
	stdinSource = strings.NewReader("old-passphrase-line\nnew-passphrase-line\n")
	stdinScanner = nil
	t.Cleanup(func() { stdinScanner = nil })

	first, err := readPassphraseLine()
	if err != nil {
		t.Fatalf("read first line: %v", err)
	}
	if first != "old-passphrase-line" {
		t.Fatalf("expected %q, got %q", "old-passphrase-line", first)
	}

	second, err := readPassphraseLine()
	if err != nil {
		t.Fatalf("read second line: %v", err)
	}
	if second != "new-passphrase-line" {
		t.Fatalf("expected %q, got %q", "new-passphrase-line", second)
	}
}

func TestReadPassphraseLineErrorsOnExhaustedInput(t *testing.T) {
	stdinSource = strings.NewReader("only-one-line\n")
	stdinScanner = nil
	t.Cleanup(func() { stdinScanner = nil })

	if _, err := readPassphraseLine(); err != nil {
		t.Fatalf("read the only line: %v", err)
	}
	if _, err := readPassphraseLine(); err == nil {
		t.Fatalf("expected an error once stdin is exhausted")
	}
}
