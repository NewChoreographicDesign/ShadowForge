package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// runZKSetup runs a real Groth16 trusted setup for the Transfer circuit
// once and saves it to -out, so a validator node (cmd/node's own
// -zk-params flag) and every wallet's "transfer" command can load the
// exact same real proving/verifying keys — see zk.System.WriteTo's own
// doc on why an independent setup per process can never interoperate.
// This is still a development setup (gnark's local, unaudited Setup),
// not a production ceremony — see pkg/zk's own doc.
func runZKSetup(args []string) error {
	fs := flag.NewFlagSet("zk-setup", flag.ExitOnError)
	out := fs.String("out", "zk-params.bin", "where to write the real Groth16 parameters")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite an existing, possibly already-shared params file", *out)
	}

	fmt.Println("running Groth16 trusted setup (development setup — see pkg/zk doc for the production-ceremony requirement)...")
	start := time.Now()
	sys, err := zk.Setup()
	if err != nil {
		return fmt.Errorf("zk setup: %w", err)
	}
	fmt.Printf("setup complete in %s\n", time.Since(start))

	f, err := os.Create(*out)
	if err != nil {
		return fmt.Errorf("create %s: %w", *out, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := sys.WriteTo(f); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Printf("wrote %s — point every validator node (-zk-params) and every wallet transfer (-zk-params) at this same file\n", *out)
	return nil
}

// loadZKSystem loads real, previously-generated Groth16 parameters from
// path — see runZKSetup's own doc on why "transfer" never generates its
// own: nothing could ever verify a proof built under a wallet's own,
// unshared setup.
func loadZKSystem(path string) (*zk.System, error) {
	if path == "" {
		return nil, fmt.Errorf("-zk-params is required: a wallet must prove against the exact same Groth16 parameters the network's validators verify against (run 'wallet zk-setup' once to generate a shared params file if one doesn't exist yet)")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open zk params file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sys, err := zk.ReadSystem(f)
	if err != nil {
		return nil, fmt.Errorf("load zk params from %s: %w", path, err)
	}
	return sys, nil
}

// runEligibilityZKSetup is runZKSetup's counterpart for the anonymous
// voter-eligibility circuit (pkg/zk.EligibilityCircuit) — a separate
// Groth16 setup and shared params file, since it's a distinct circuit
// from TransferCircuit and needs its own proving/verifying keys every
// validator and voting wallet must agree on.
func runEligibilityZKSetup(args []string) error {
	fs := flag.NewFlagSet("eligibility-zk-setup", flag.ExitOnError)
	out := fs.String("out", "eligibility-zk-params.bin", "where to write the real Groth16 parameters")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite an existing, possibly already-shared params file", *out)
	}

	fmt.Println("running Groth16 trusted setup for anonymous voter eligibility (development setup — see pkg/zk doc for the production-ceremony requirement)...")
	start := time.Now()
	sys, err := zk.SetupEligibility()
	if err != nil {
		return fmt.Errorf("eligibility zk setup: %w", err)
	}
	fmt.Printf("setup complete in %s\n", time.Since(start))

	f, err := os.Create(*out)
	if err != nil {
		return fmt.Errorf("create %s: %w", *out, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := sys.WriteTo(f); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Printf("wrote %s — point every validator node (-eligibility-zk-params) and every wallet vote/vote-reveal (-eligibility-zk-params) at this same file\n", *out)
	return nil
}

// loadEligibilitySystem loads real, previously-generated Groth16
// parameters for the eligibility circuit from path — see loadZKSystem's
// own doc for why this must always be a shared file, never a fresh
// per-process setup.
func loadEligibilitySystem(path string) (*zk.EligibilitySystem, error) {
	if path == "" {
		return nil, fmt.Errorf("-eligibility-zk-params is required: a wallet must prove against the exact same Groth16 parameters the network's validators verify against (run 'wallet eligibility-zk-setup' once to generate a shared params file if one doesn't exist yet)")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open eligibility zk params file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sys, err := zk.ReadEligibilitySystem(f)
	if err != nil {
		return nil, fmt.Errorf("load eligibility zk params from %s: %w", path, err)
	}
	return sys, nil
}
