// Command walletkey is a real, standalone CLI for pkg/walletkey — the
// end-user-facing tool this session's Tier B priority #2 (real local key
// management) actually needs to be usable by a person, not just a
// library. It never runs consensus, never opens a network listener, and
// never takes a passphrase as a command-line argument (which would leak
// it into shell history and the process list on any multi-user machine)
// — a real terminal prompt (masked, via golang.org/x/term) is the default
// input path, with an explicit -passphrase-stdin flag as the only
// scriptable alternative, still kept out of argv.
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "show":
		err = runShow(args)
	case "sign":
		err = runSign(args)
	case "passwd":
		err = runPasswd(args)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "walletkey: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `walletkey — a real, passphrase-protected local identity for ShadowForge.

Usage:
  walletkey init   -path <file>                    create a new keystore
  walletkey show   -path <file>                     print its public identity (no passphrase needed)
  walletkey sign   -path <file> -message <text>     unlock, sign <text>, verify the result
  walletkey passwd -path <file>                     change the keystore's passphrase

All commands prompt for a passphrase on the real terminal (input hidden).
Pass -passphrase-stdin to read it from stdin instead, one line, for
scripting — never pass a passphrase as a plain command-line argument.`)
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	path := fs.String("path", "walletkey.json", "where to write the new keystore")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*path); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite an existing keystore", *path)
	}

	var passphrase string
	var err error
	if *fromStdin {
		passphrase, err = readPassphraseLine()
	} else {
		passphrase, err = promptNewPassphrase()
	}
	if err != nil {
		return err
	}

	ks, err := walletkey.Generate(passphrase)
	if err != nil {
		return err
	}
	if err := ks.Save(*path); err != nil {
		return err
	}

	fmt.Printf("created %s\n", *path)
	fmt.Printf("identity: %s\n", ks.Identity())
	fmt.Printf("public key: %s\n", ks.PublicKey())
	return nil
}

func runShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	path := fs.String("path", "walletkey.json", "keystore to read")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}
	fmt.Printf("identity: %s\n", ks.Identity())
	fmt.Printf("public key: %s\n", ks.PublicKey())
	return nil
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	path := fs.String("path", "walletkey.json", "keystore to unlock")
	message := fs.String("message", "", "the message to sign (required)")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *message == "" {
		return fmt.Errorf("-message is required")
	}

	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}

	passphrase, err := readExistingPassphrase(*fromStdin)
	if err != nil {
		return err
	}

	pk, sk, err := ks.Unlock(passphrase)
	if err != nil {
		return err
	}

	sig, err := crypto.DilithiumSign(sk, []byte(*message))
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	ok, err := crypto.DilithiumVerify(pk, []byte(*message), sig)
	if err != nil || !ok {
		return fmt.Errorf("SAFETY: the signature this identity just produced did not verify against its own public key (ok=%v err=%v) — refusing to print a signature that doesn't check out", ok, err)
	}

	fmt.Printf("identity: %s\n", ks.Identity())
	fmt.Printf("message: %s\n", *message)
	fmt.Printf("signature (verified): %s\n", hex.EncodeToString(sig))
	return nil
}

func runPasswd(args []string) error {
	fs := flag.NewFlagSet("passwd", flag.ExitOnError)
	path := fs.String("path", "walletkey.json", "keystore to update")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the old passphrase then the new one from stdin (two lines, no confirmation) instead of prompting the terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ks, err := walletkey.Load(*path)
	if err != nil {
		return err
	}

	var oldPass, newPass string
	if *fromStdin {
		if oldPass, err = readPassphraseLine(); err != nil {
			return err
		}
		if newPass, err = readPassphraseLine(); err != nil {
			return err
		}
		if newPass == "" {
			return fmt.Errorf("new passphrase must not be empty")
		}
	} else {
		if oldPass, err = promptMasked("current passphrase: "); err != nil {
			return err
		}
		if newPass, err = promptNewPassphrase(); err != nil {
			return err
		}
	}

	if err := ks.ChangePassphrase(oldPass, newPass); err != nil {
		return err
	}
	if err := ks.Save(*path); err != nil {
		return err
	}
	fmt.Printf("passphrase changed for %s\n", *path)
	return nil
}

// readExistingPassphrase gets a passphrase for unlocking an existing
// keystore — a single read, no confirmation (there's nothing to confirm
// against; a wrong entry is simply rejected by Unlock).
func readExistingPassphrase(fromStdin bool) (string, error) {
	if fromStdin {
		return readPassphraseLine()
	}
	return promptMasked("passphrase: ")
}

// promptNewPassphrase asks for a new passphrase twice on the real
// terminal and requires them to match, so a typo doesn't silently lock a
// person out of a keystore they just created or just rotated into.
func promptNewPassphrase() (string, error) {
	first, err := promptMasked("new passphrase: ")
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", fmt.Errorf("passphrase must not be empty")
	}
	second, err := promptMasked("confirm passphrase: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("passphrases did not match")
	}
	return first, nil
}

// promptMasked reads one line from the real controlling terminal with
// input echo disabled, so a passphrase never appears on screen. It
// deliberately does not fall back to a plaintext read when stdin isn't a
// terminal — a caller in that situation (a script, a pipe) should use
// -passphrase-stdin instead, which makes the non-interactive intent
// explicit rather than silently degrading a "hidden input" guarantee.
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
// process, created lazily on first use. A bufio.Scanner reads from its
// underlying source in chunks, not one line at a time — a fresh scanner
// per call (the first version of this function did that) silently
// discards whatever it over-read past the first newline the moment it
// goes out of scope, so a command like passwd that needs two sequential
// lines from stdin (old passphrase, new passphrase) would lose the second
// one even though it was sitting right there in the pipe. One scanner,
// created once, avoids that entirely.
var stdinScanner *bufio.Scanner

// readPassphraseLine reads exactly one (further) line from stdin — the
// -passphrase-stdin path, for scripting. Kept separate from promptMasked
// so scripted use never depends on stdin being a terminal.
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
