// Command shadowc is the ShadowRust CLI: parse, lint, interp, gen (spec
// 18.1 repository layout, 18.3 Phase 1 deliverable).
package main

import (
	"fmt"
	"os"

	"github.com/shadowforge/shadowforge-l1/ast"
	"github.com/shadowforge/shadowforge-l1/codegen"
	"github.com/shadowforge/shadowforge-l1/interp"
	"github.com/shadowforge/shadowforge-l1/sema"
)

func usage() {
	fmt.Fprintln(os.Stderr, `shadowc - ShadowRust CLI

Usage:
  shadowc parse  <file.sr>            print the parsed AST
  shadowc lint   <file.sr>            run the semantic analyzer
  shadowc interp <file.sr>            run the sandbox interpreter
  shadowc gen    <file.sr> [pkgname]  emit generated Go (default package "generated")`)
}

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	cmd, path := os.Args[1], os.Args[2]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shadowc:", err)
		os.Exit(1)
	}

	prog, errs := ast.Parse(string(src))
	if len(errs) != 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "syntax error:", e.String())
		}
		os.Exit(1)
	}

	switch cmd {
	case "parse":
		fmt.Printf("%d top-level statement(s) parsed OK\n", len(prog.Statements))
		for i, s := range prog.Statements {
			fmt.Printf("  [%d] %T\n", i, s)
		}

	case "lint":
		diags := sema.Analyze(prog)
		if len(diags) == 0 {
			fmt.Println("lint: no issues found")
			return
		}
		for _, d := range diags {
			fmt.Fprintln(os.Stderr, "lint:", d.String())
		}
		os.Exit(1)

	case "interp":
		if diags := sema.Analyze(prog); len(diags) != 0 {
			for _, d := range diags {
				fmt.Fprintln(os.Stderr, "lint:", d.String())
			}
			os.Exit(1)
		}
		it := interp.New(nil)
		if err := it.Run(prog); err != nil {
			fmt.Fprintln(os.Stderr, "interp error:", err)
			os.Exit(1)
		}
		fmt.Printf("txs=%d feeRoutes=%d deposits=%d queueInserts=%d\n",
			len(it.Txs), len(it.FeeRoutes), len(it.Deposits), len(it.QueueInsert))

	case "gen":
		if diags := sema.Analyze(prog); len(diags) != 0 {
			for _, d := range diags {
				fmt.Fprintln(os.Stderr, "lint:", d.String())
			}
			os.Exit(1)
		}
		pkgName := "generated"
		if len(os.Args) > 3 {
			pkgName = os.Args[3]
		}
		out, err := codegen.Generate(pkgName, prog)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gen error:", err)
			os.Exit(1)
		}
		fmt.Print(out)

	default:
		usage()
		os.Exit(2)
	}
}
