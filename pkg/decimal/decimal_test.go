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
