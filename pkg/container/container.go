// Package container implements the enterprise L1 container: a shielded
// subspace that mirrors a business's internal server (spec section 15.3,
// 16). A container runs the full five-stage pipeline for internal traffic
// and periodically aggregates a mega-batch into the public revolver as a
// Kind ContainerSync transaction.
package container

import (
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Config is a container's static configuration (spec 15.3).
type Config struct {
	ID                 types.ID
	Departments        []string
	ValidatorsBase     int             // base 20 validators
	ValidatorsPerDept  int             // plus 2 per department
	HybridSplitPercent decimal.Decimal // 50% internal, 50% also join the public revolver
	SyncTPSThreshold   int             // example: 1,000
	SyncInterval       time.Duration   // example: 5 minutes
}

// DefaultConfig fills in the spec 15.3 example numbers for fields the
// caller leaves zero.
func DefaultConfig(id types.ID, departments []string) Config {
	return Config{
		ID:                 id,
		Departments:        departments,
		ValidatorsBase:     20,
		ValidatorsPerDept:  2,
		HybridSplitPercent: decimal.MustFromString("0.50"),
		SyncTPSThreshold:   1000,
		SyncInterval:       5 * time.Minute,
	}
}

// RequiredValidators implements spec 15.3: "Base 20 validators plus 2 per
// department."
func RequiredValidators(cfg Config) int {
	return cfg.ValidatorsBase + cfg.ValidatorsPerDept*len(cfg.Departments)
}

// HybridSplitCounts implements the 50/50 hybrid rule: half the container's
// validators process internal traffic, half also join the public revolver.
func HybridSplitCounts(cfg Config) (internal, public int) {
	total := RequiredValidators(cfg)
	publicF := decimal.FromInt(int64(total)).Mul(cfg.HybridSplitPercent)
	public = int(publicF.Uint64())
	return total - public, public
}

// Subspace is the runtime state of one enterprise container.
type Subspace struct {
	cfg Config

	mu           sync.Mutex
	localTxSince int
	lastSyncAt   time.Time
	internalMode bool
	whitelist    map[string]bool // dept/workflow names exempt from silent-TX DoS holds (spec 16.6)
}

func NewSubspace(cfg Config) *Subspace {
	return &Subspace{cfg: cfg, lastSyncAt: time.Now(), whitelist: map[string]bool{}}
}

func (s *Subspace) Config() Config { return s.cfg }

// RecordLocalTx counts one internally-processed transaction toward the
// next sync decision.
func (s *Subspace) RecordLocalTx() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localTxSince++
}

// ShouldSync implements spec 15.3: "When internal TPS exceeds a threshold
// (example: 1,000) or an interval elapses (example: 5 minutes), the
// container aggregates a mega-batch and submits it as Kind ContainerSync."
func (s *Subspace) ShouldSync(now time.Time, currentTPS int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return currentTPS > s.cfg.SyncTPSThreshold || now.Sub(s.lastSyncAt) >= s.cfg.SyncInterval
}

// MarkSynced resets the sync-window counters after a ContainerSync
// transaction has been submitted.
func (s *Subspace) MarkSynced(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localTxSince = 0
	s.lastSyncAt = now
}

// EnterInternalMode / ExitInternalMode implement spec 5.6: "Enterprise
// containers that cannot reach the public network flip to internal mode:
// 100 percent of their local validators process local traffic only."
func (s *Subspace) EnterInternalMode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.internalMode = true
}

func (s *Subspace) ExitInternalMode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.internalMode = false
}

func (s *Subspace) InternalMode() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.internalMode
}

// Whitelist marks a workflow/department name exempt from silent-TX DoS
// holds, so a payroll burst is not treated as an attack (spec 15.4, 16.6).
func (s *Subspace) Whitelist(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.whitelist[name] = true
}

func (s *Subspace) IsWhitelisted(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.whitelist[name]
}

// ShadowVerify implements spec 16.3: "Shadow verification compares the
// container output to the duplicate server. Mismatch blocks commit."
func ShadowVerify(containerOutput, duplicateOutput types.Hash) bool {
	return containerOutput == duplicateOutput
}

// Blueprint is the JSON shape Phase 1 assessment produces (spec 16.1).
type Blueprint struct {
	Depts     []DeptBlueprint     `json:"depts"`
	Workflows []WorkflowBlueprint `json:"workflows"`
}

type DeptBlueprint struct {
	Name   string   `json:"name"`
	Traits []string `json:"traits"`
}

type WorkflowBlueprint struct {
	From string `json:"from"`
	To   string `json:"to"` // e.g. "tx update_trait finance balance += 500"
}
