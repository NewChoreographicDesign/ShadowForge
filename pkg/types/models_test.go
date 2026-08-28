package types_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestMintNetAmountAppliesRealTenPercentFee(t *testing.T) {
	if got := types.MintNetAmount(1000); got != 900 {
		t.Fatalf("MintNetAmount(1000) = %d, want 900", got)
	}
	if got := types.MintFeeAmount(1000); got != 100 {
		t.Fatalf("MintFeeAmount(1000) = %d, want 100", got)
	}
}

func TestMintNetAmountFloorsOnUnevenDivision(t *testing.T) {
	// 1005 / 10 = 100.5 -> fee floors to 100, net gets the extra unit.
	if got := types.MintFeeAmount(1005); got != 100 {
		t.Fatalf("MintFeeAmount(1005) = %d, want 100 (floor)", got)
	}
	if got := types.MintNetAmount(1005); got != 905 {
		t.Fatalf("MintNetAmount(1005) = %d, want 905", got)
	}
}

func TestMintNetAmountAndFeeAmountSumToOriginal(t *testing.T) {
	for _, amount := range []uint64{0, 1, 9, 10, 11, 1000, 123456789} {
		net := types.MintNetAmount(amount)
		fee := types.MintFeeAmount(amount)
		if net+fee != amount {
			t.Fatalf("amount %d: net %d + fee %d = %d, want %d", amount, net, fee, net+fee, amount)
		}
	}
}
