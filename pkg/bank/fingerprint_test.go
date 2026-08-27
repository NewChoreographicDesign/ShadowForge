package bank_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
)

func TestIPFingerprintDeterministicAndDistinct(t *testing.T) {
	salt := bank.DaySalt(time.Now())
	fp := bank.IPFingerprint("203.0.113.42", salt)

	// Deterministic for the same (ip, salt) pair.
	fp2 := bank.IPFingerprint("203.0.113.42", salt)
	if fp != fp2 {
		t.Fatalf("expected the same (ip, salt) to fingerprint identically")
	}

	// Distinct for a different IP under the same salt — the fingerprint
	// carries no collision that would defeat its purpose.
	other := bank.IPFingerprint("198.51.100.7", salt)
	if fp == other {
		t.Fatalf("different IPs must not collide to the same fingerprint")
	}
}

func TestIPFingerprintRotatesDaily(t *testing.T) {
	day1 := bank.DaySalt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	day2 := bank.DaySalt(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	fp1 := bank.IPFingerprint("203.0.113.42", day1)
	fp2 := bank.IPFingerprint("203.0.113.42", day2)
	if fp1 == fp2 {
		t.Fatalf("expected different days to produce different fingerprints for the same IP")
	}
}

func TestCorrelationTrackerFlagsRepeatedFlows(t *testing.T) {
	c := bank.NewCorrelationTracker()
	salt := bank.DaySalt(time.Now())
	var fp = mustFingerprint(t, c, "198.51.100.7", salt, 5)
	if c.IsCorrelated(fp, 3) != true {
		t.Fatalf("5 flows should exceed a threshold of 3")
	}
	if c.IsCorrelated(fp, 10) {
		t.Fatalf("5 flows should not exceed a threshold of 10")
	}
}

func mustFingerprint(t *testing.T, c *bank.CorrelationTracker, ip string, salt []byte, n int) (fp [32]byte) {
	t.Helper()
	for i := 0; i < n; i++ {
		fp, _ = c.Record(ip, salt)
	}
	return fp
}

func TestCorrelationTrackerDistinctIPsIndependent(t *testing.T) {
	c := bank.NewCorrelationTracker()
	salt := bank.DaySalt(time.Now())
	fpA, countA := c.Record("198.51.100.1", salt)
	fpB, countB := c.Record("198.51.100.2", salt)
	if fpA == fpB {
		t.Fatalf("different IPs must not collide to the same fingerprint")
	}
	if countA != 1 || countB != 1 {
		t.Fatalf("expected independent counts, got %d and %d", countA, countB)
	}
}
