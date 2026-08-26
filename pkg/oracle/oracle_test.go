package oracle_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func dec(s string) decimal.Decimal { return decimal.MustFromString(s) }

func TestQuorumAgreesWithinBound(t *testing.T) {
	q := oracle.NewQuorum(dec("0.02"),
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("60000"), ATRUSD: dec("2000")}},
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("60500"), ATRUSD: dec("2010")}},
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("59800"), ATRUSD: dec("1990")}},
	)
	got, err := q.Quote(types.AssetBTC)
	if err != nil {
		t.Fatalf("expected agreement, got error: %v", err)
	}
	if got.PriceUSD.Cmp(dec("60000")) != 0 {
		t.Fatalf("expected median price 60000, got %s", got.PriceUSD)
	}
}

func TestQuorumFreezesOnDisagreement(t *testing.T) {
	q := oracle.NewQuorum(dec("0.02"),
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("60000"), ATRUSD: dec("2000")}},
		oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("90000"), ATRUSD: dec("2000")}}, // 50% off
	)
	if _, err := q.Quote(types.AssetBTC); err != oracle.ErrDisagreement {
		t.Fatalf("expected ErrDisagreement, got %v", err)
	}
}

func TestQuorumLastGoodPersistsAfterFreeze(t *testing.T) {
	src1 := oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("60000"), ATRUSD: dec("2000"), Timestamp: 1}}
	src2 := oracle.StaticSource{Value: oracle.Quote{PriceUSD: dec("60100"), ATRUSD: dec("2000"), Timestamp: 1}}
	q := oracle.NewQuorum(dec("0.02"), src1, src2)
	if _, err := q.Quote(types.AssetBTC); err != nil {
		t.Fatalf("expected agreement: %v", err)
	}
	lastGood, ok := q.LastGood(types.AssetBTC)
	if !ok {
		t.Fatalf("expected a last-good quote to be recorded")
	}
	if lastGood.PriceUSD.Sign() <= 0 {
		t.Fatalf("expected a positive last-good price")
	}
}
