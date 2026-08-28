package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// These passphrase-prompt helpers are deliberately duplicated from
// cmd/walletkey rather than shared through a new internal package — the
// same "each binary a single, independent file" precedent cmd/walletsim's
// own waitForAddrFile helper already set in this codebase.

// readExistingPassphrase gets a passphrase for unlocking an existing
// keystore — a single read, no confirmation (there's nothing to confirm
// against; a wrong entry is simply rejected by Unlock/UnlockShielded).
func readExistingPassphrase(fromStdin bool) (string, error) {
	if fromStdin {
		return readPassphraseLine()
	}
	return promptMasked("passphrase: ")
}

// promptMasked reads one line from the real controlling terminal with
// input echo disabled, so a passphrase never appears on screen. It
// deliberately does not fall back to a plaintext read when stdin isn't a
// terminal — a caller in that situation should use -passphrase-stdin
// instead, which makes the non-interactive intent explicit rather than
// silently degrading a "hidden input" guarantee.
func promptMasked(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("stdin is not a terminal — use -passphrase-stdin to provide a passphrase non-interactively")
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	return string(b), nil
}

// stdinSource is where readPassphraseLine reads from — os.Stdin in the
// real binary, swapped out by tests.
var stdinSource io.Reader = os.Stdin

// stdinScanner is shared across every readPassphraseLine call in this
// process, created lazily on first use — see cmd/walletkey's identical
// helper for why a fresh scanner per call would silently lose buffered
// input.
var stdinScanner *bufio.Scanner

// readPassphraseLine reads exactly one (further) line from stdin — the
// -passphrase-stdin path, for scripting.
func readPassphraseLine() (string, error) {
	if stdinScanner == nil {
		stdinScanner = bufio.NewScanner(stdinSource)
	}
	if !stdinScanner.Scan() {
		if err := stdinScanner.Err(); err != nil {
			return "", fmt.Errorf("read passphrase from stdin: %w", err)
		}
		return "", fmt.Errorf("no passphrase line found on stdin")
	}
	return stdinScanner.Text(), nil
}
