package decimal_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
)

func TestBasicMath(t *testing.T) {
	a := decimal.MustFromString("2.5")
	b := decimal.FromInt(100)
	got := a.Mul(b)
	want := decimal.MustFromString("250")
	if got.Cmp(want) != 0 {
		t.Fatalf("2.5*100 = %s, want %s", got, want)
	}
}

func TestMax0(t *testing.T) {
	neg := decimal.MustFromString("-5")
	if neg.Max0().Sign() != 0 {
		t.Fatalf("Max0 of -5 should be 0, got %s", neg.Max0())
	}
	pos := decimal.MustFromString("5")
	if pos.Max0().Cmp(pos) != 0 {
		t.Fatalf("Max0 of 5 should be 5, got %s", pos.Max0())
	}
}

func TestDivByZeroPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on division by zero")
		}
	}()
	decimal.FromInt(1).Div(decimal.Zero)
}

func TestNew(t *testing.T) {
	got := decimal.New(5, 2) // 5/2 = 2.5
	if got.Cmp(decimal.MustFromString("2.5")) != 0 {
		t.Fatalf("New(5,2) = %s, want 2.5", got)
	}
}

func TestIsNeg(t *testing.T) {
	if !decimal.MustFromString("-1").IsNeg() {
		t.Fatalf("-1 should be negative")
	}
	if decimal.MustFromString("1").IsNeg() {
		t.Fatalf("1 should not be negative")
	}
	if decimal.Zero.IsNeg() {
		t.Fatalf("0 should not be negative")
	}
}

func TestMax(t *testing.T) {
	a, b := decimal.FromInt(3), decimal.FromInt(7)
	if decimal.Max(a, b).Cmp(b) != 0 {
		t.Fatalf("Max(3,7) should be 7")
	}
	if decimal.Max(b, a).Cmp(b) != 0 {
		t.Fatalf("Max(7,3) should be 7")
	}
}

func TestFloat64(t *testing.T) {
	got := decimal.MustFromString("2.5").Float64()
	if got != 2.5 {
		t.Fatalf("Float64() = %v, want 2.5", got)
	}
}

func TestFromStringInvalid(t *testing.T) {
	if _, err := decimal.FromString("not-a-number"); err == nil {
		t.Fatalf("expected an error for an invalid decimal literal")
	}
}

func TestMustFromStringPanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for an invalid literal")
		}
	}()
	decimal.MustFromString("not-a-number")
}

func TestJSONRoundTrip(t *testing.T) {
	d := decimal.MustFromString("5/2")
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got decimal.Decimal
	if err := got.UnmarshalJSON(b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Cmp(d) != 0 {
		t.Fatalf("round trip mismatch: got %s want %s", got, d)
	}
}

func TestUnmarshalJSONInvalid(t *testing.T) {
	var d decimal.Decimal
	if err := d.UnmarshalJSON([]byte("not-quoted")); err == nil {
		t.Fatalf("expected error for a non-string JSON value")
	}
	if err := d.UnmarshalJSON([]byte(`"garbage"`)); err == nil {
		t.Fatalf("expected error for an unparseable quoted value")
	}
}

func TestZeroValueDecimalBehavesAsZero(t *testing.T) {
	var d decimal.Decimal
	if d.Sign() != 0 {
		t.Fatalf("zero-value Decimal should have Sign()==0")
	}
	if d.String() != "0" {
		t.Fatalf("zero-value Decimal should stringify as 0, got %q", d.String())
	}
}
