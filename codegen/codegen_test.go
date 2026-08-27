package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shadowforge/shadowforge-l1/ast"
	"github.com/shadowforge/shadowforge-l1/codegen"
	"github.com/shadowforge/shadowforge-l1/sema"
)

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if diags := sema.Analyze(prog); len(diags) != 0 {
		t.Fatalf("sema diagnostics: %v", diags)
	}
	return prog
}

// buildGenerated writes src into a throwaway package inside this module
// (so it can import codegen/runtime with its real module path) and runs
// `go build` against it, failing the test on any compile error.
func buildGenerated(t *testing.T, src string) {
	t.Helper()
	dir := filepath.Join("zz_gentest_" + sanitizeName(t.Name()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "gen.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("go", "build", "./"+dir)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated code failed to compile: %v\n%s\n---\n%s", err, out, src)
	}
}

func sanitizeName(s string) string {
	return strings.NewReplacer("/", "_", " ", "_").Replace(s)
}

func TestGenerateSampleTxCompilesAndCallsTransfer(t *testing.T) {
	prog := mustParse(t, `tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05 to vault_address;
}
`)
	src, err := codegen.Generate("gentest", prog)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, `node.Transfer("sender", "receiver", amount, "vault_address", project_fee)`) {
		t.Fatalf("generated source missing expected Transfer call:\n%s", src)
	}
	buildGenerated(t, src)
}

func TestGenerateBankDepositCompiles(t *testing.T) {
	prog := mustParse(t, `bank deposit hold1 atr atr_feed;`)
	src, err := codegen.Generate("gentest", prog)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "node.BankDeposit(\"hold1\", atr_feed)") {
		t.Fatalf("generated source missing expected BankDeposit call:\n%s", src)
	}
	buildGenerated(t, src)
}

func TestGenerateQueueInsertCompiles(t *testing.T) {
	prog := mustParse(t, `queue insert validator1 positions 4, 10, 2, 7;`)
	src, err := codegen.Generate("gentest", prog)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(src, "node.QueueInsert(\"validator1\", []int{4, 10, 2, 7})") {
		t.Fatalf("generated source missing expected QueueInsert call:\n%s", src)
	}
	buildGenerated(t, src)
}

func TestGenerateMintAndUpdateTraitCompile(t *testing.T) {
	prog := mustParse(t, `mint proposer1 amount 1000 epoch 5;
update_trait finance balance += 500;
`)
	src, err := codegen.Generate("gentest", prog)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	buildGenerated(t, src)
}

func TestGenerateNestedTxRejected(t *testing.T) {
	// codegen v0.1 scope: a tx nested inside another tx must be a hard
	// error, not a silent miscompile (see codegen.go package doc).
	src := `tx buy outer from a to b amount 10 {
    tx buy inner from c to d amount 5 {
        fee = 1 to vault;
    }
}
`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if _, err := codegen.Generate("gentest", prog); err == nil {
		t.Fatalf("expected codegen error for nested tx")
	}
}
