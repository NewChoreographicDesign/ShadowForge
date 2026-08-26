package consensus

import "sync"

// SentinelCount is the fixed number of protocol-run sentinel validators
// (spec 5.5): "If the number of online, non-cooldown validators in the
// revolver is strictly less than 10, the protocol activates 10 sentinel
// validators."
const SentinelCount = 10

// SentinelManager tracks sentinel activation state and the activation-count
// metric spec 5.5 and 3.4 name as a first-class health bar ("Year-1 budget:
// fewer than 5 activations per month").
type SentinelManager struct {
	mu          sync.Mutex
	active      bool
	activations []int64 // unix-ms timestamps of each activation, for the monthly-rate metric
}

func NewSentinelManager() *SentinelManager {
	return &SentinelManager{}
}

// Active reports whether sentinels are currently processing stages.
func (s *SentinelManager) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Evaluate applies spec 19.6 (`if revolver.Online() < 10 { activateSentinels(10) }`)
// and, symmetrically, spec 5.5's withdrawal rule ("When the civilian queue
// recovers to 10 or more, sentinels withdraw in an orderly way after
// finishing the current batch"). It returns the action the caller must take
// this tick; the caller is responsible for actually letting an in-flight
// batch finish before acting on ActionWithdraw.
type SentinelAction uint8

const (
	ActionNone SentinelAction = iota
	ActionActivate
	ActionWithdraw
)

func (s *SentinelManager) Evaluate(onlineCivilians int, nowUnixMilli int64) SentinelAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	needed := onlineCivilians < SentinelThreshold
	switch {
	case needed && !s.active:
		s.active = true
		s.activations = append(s.activations, nowUnixMilli)
		return ActionActivate
	case !needed && s.active:
		s.active = false
		return ActionWithdraw
	default:
		return ActionNone
	}
}

// ActivationsInWindow counts activations within [since, now], for the
// "fewer than 5 activations per month" health metric (spec 3.4).
func (s *SentinelManager) ActivationsInWindow(since, now int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range s.activations {
		if t >= since && t <= now {
			n++
		}
	}
	return n
}
