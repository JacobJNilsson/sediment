// Package decay implements confidence decay for sediment memories.
//
// The decay model follows the article's exponential decay formula:
//
//	confidence(t) = initial * e^(-lambda * hours_since_last_access)
//
// where lambda controls how fast a memory fades. Frequently accessed
// memories resist erosion through reinforcement: each access boosts
// confidence back up.
package decay

import (
	"math"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
)

// DefaultLambda is the default decay rate per hour.
// A value of 0.01 means ~1% decay per hour, or roughly halving in 69 hours.
const DefaultLambda = 0.01

// Config holds tunable parameters for the decay engine.
type Config struct {
	// Lambda is the exponential decay rate per hour.
	Lambda float64
	// DormancyThreshold: memories below this confidence become dormant.
	DormancyThreshold float64
	// ArchiveThreshold: memories below this confidence become archived.
	ArchiveThreshold float64
	// ReinforcementBoost: confidence added on each access (capped at 1.0).
	ReinforcementBoost float64
}

// DefaultConfig returns sensible defaults matching the article's recommendations.
func DefaultConfig() Config {
	return Config{
		Lambda:             DefaultLambda,
		DormancyThreshold:  0.4,
		ArchiveThreshold:   0.1,
		ReinforcementBoost: 0.15,
	}
}

// EffectiveLambda adjusts the base decay rate by hardness.
// Harder memories decay more slowly: lambda_eff = lambda / hardness.
func EffectiveLambda(lambda float64, h model.Hardness) float64 {
	if h < model.HardnessMin {
		h = model.HardnessDefault
	}
	return lambda / float64(h)
}

// CurrentConfidence computes the decayed confidence at the given time.
func CurrentConfidence(m *model.Memory, now time.Time, lambda float64) float64 {
	hours := now.Sub(m.LastAccessedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	effectiveLambda := EffectiveLambda(lambda, m.Hardness)
	decayed := m.Confidence * math.Exp(-effectiveLambda*hours)
	return clamp(decayed, 0, 1)
}

// Reinforce boosts a memory's confidence after an access.
//
// Reinforce mutates m in-place: it first decays Confidence to the current time,
// then adds ReinforcementBoost (clamped to [0,1]), and finally updates
// AccessCount, LastAccessedAt, and UpdatedAt.
//
// If you need the pre-reinforcement (decayed-only) confidence value, call
// CurrentConfidence before Reinforce, because Reinforce overwrites both
// Confidence and LastAccessedAt.
//
// Calling Reinforce twice without resetting LastAccessedAt between calls will
// produce incorrect decay calculations, since the second call decays from the
// already-updated LastAccessedAt rather than the original value.
func Reinforce(m *model.Memory, now time.Time, cfg Config) {
	m.Confidence = CurrentConfidence(m, now, cfg.Lambda)
	m.Confidence = clamp(m.Confidence+cfg.ReinforcementBoost, 0, 1)
	m.AccessCount++
	m.LastAccessedAt = now
	m.UpdatedAt = now
}

// Classify returns the state a memory should be in based on its
// current decayed confidence.
func Classify(confidence float64, cfg Config) model.State {
	switch {
	case confidence < cfg.ArchiveThreshold:
		return model.StateArchived
	case confidence < cfg.DormancyThreshold:
		return model.StateDormant
	default:
		return model.StateActive
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
