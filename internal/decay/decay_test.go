package decay_test

import (
	"math"
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/decay"
	"github.com/jacobjnilsson/sediment/internal/model"
)

var baseCfg = decay.DefaultConfig()

func TestCurrentConfidence_NoDecay(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{Confidence: 0.8, LastAccessedAt: now}

	got := decay.CurrentConfidence(m, now, baseCfg.Lambda)
	if math.Abs(got-0.8) > 1e-9 {
		t.Errorf("CurrentConfidence = %v, want 0.8 (no time elapsed)", got)
	}
}

func TestCurrentConfidence_AfterHours(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{Confidence: 1.0, LastAccessedAt: now.Add(-24 * time.Hour)}

	got := decay.CurrentConfidence(m, now, baseCfg.Lambda)
	// 1.0 * e^(-0.01 * 24) = e^(-0.24) ≈ 0.7866
	want := math.Exp(-0.24)
	if math.Abs(got-want) > 1e-4 {
		t.Errorf("CurrentConfidence = %v, want ~%v", got, want)
	}
}

func TestCurrentConfidence_FutureTime(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{Confidence: 0.5, LastAccessedAt: now.Add(1 * time.Hour)}

	// Future access time should not increase confidence (hours clamped to 0).
	got := decay.CurrentConfidence(m, now, baseCfg.Lambda)
	if math.Abs(got-0.5) > 1e-9 {
		t.Errorf("CurrentConfidence = %v, want 0.5 (future time)", got)
	}
}

func TestCurrentConfidence_VeryOld(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{Confidence: 1.0, LastAccessedAt: now.Add(-10000 * time.Hour)}

	got := decay.CurrentConfidence(m, now, baseCfg.Lambda)
	if got < 0 {
		t.Errorf("CurrentConfidence = %v, want >= 0", got)
	}
	if got > 0.01 {
		t.Errorf("CurrentConfidence = %v, want near 0 for very old memory", got)
	}
}

func TestReinforce(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{
		Confidence:     0.5,
		AccessCount:    2,
		LastAccessedAt: now.Add(-10 * time.Hour),
		UpdatedAt:      now.Add(-10 * time.Hour),
	}

	decay.Reinforce(m, now, baseCfg)

	// After 10h at lambda=0.01: 0.5 * e^(-0.1) ≈ 0.4524, + 0.15 boost = ~0.6024
	wantMin := 0.55
	wantMax := 0.65
	if m.Confidence < wantMin || m.Confidence > wantMax {
		t.Errorf("Confidence = %v, want in [%v, %v]", m.Confidence, wantMin, wantMax)
	}
	if m.AccessCount != 3 {
		t.Errorf("AccessCount = %d, want 3", m.AccessCount)
	}
	if !m.LastAccessedAt.Equal(now) {
		t.Errorf("LastAccessedAt = %v, want %v", m.LastAccessedAt, now)
	}
	if !m.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", m.UpdatedAt, now)
	}
}

func TestReinforce_CapsAt1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{
		Confidence:     0.95,
		LastAccessedAt: now,
	}

	decay.Reinforce(m, now, baseCfg)

	if m.Confidence > 1.0 {
		t.Errorf("Confidence = %v, want <= 1.0", m.Confidence)
	}
	if m.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 (0.95 + 0.15 capped)", m.Confidence)
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		confidence float64
		want       model.State
	}{
		{"high confidence is active", 0.8, model.StateActive},
		{"at dormancy threshold is active", 0.4, model.StateActive},
		{"just below dormancy is dormant", 0.39, model.StateDormant},
		{"at archive threshold is dormant", 0.1, model.StateDormant},
		{"below archive is archived", 0.09, model.StateArchived},
		{"zero is archived", 0.0, model.StateArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := decay.Classify(tt.confidence, baseCfg)
			if got != tt.want {
				t.Errorf("Classify(%v) = %v, want %v", tt.confidence, got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := decay.DefaultConfig()

	if cfg.Lambda != decay.DefaultLambda {
		t.Errorf("Lambda = %v, want %v", cfg.Lambda, decay.DefaultLambda)
	}
	if cfg.DormancyThreshold != 0.4 {
		t.Errorf("DormancyThreshold = %v, want 0.4", cfg.DormancyThreshold)
	}
	if cfg.ArchiveThreshold != 0.1 {
		t.Errorf("ArchiveThreshold = %v, want 0.1", cfg.ArchiveThreshold)
	}
	if cfg.ReinforcementBoost != 0.15 {
		t.Errorf("ReinforcementBoost = %v, want 0.15", cfg.ReinforcementBoost)
	}
}

func TestCurrentConfidence_NegativeResult(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// Negative confidence should be clamped to 0.
	m := &model.Memory{Confidence: -0.5, LastAccessedAt: now}
	got := decay.CurrentConfidence(m, now, 0.01)
	if got != 0 {
		t.Errorf("CurrentConfidence = %v, want 0 (clamped)", got)
	}
}

func TestReinforce_HighBoostClamped(t *testing.T) {
	t.Parallel()
	now := time.Now()
	m := &model.Memory{
		Confidence:     0.99,
		LastAccessedAt: now,
	}
	// Use a very high boost to ensure clamping at 1.0.
	cfg := decay.Config{
		Lambda:             0.01,
		DormancyThreshold:  0.4,
		ArchiveThreshold:   0.1,
		ReinforcementBoost: 0.5,
	}
	decay.Reinforce(m, now, cfg)
	if m.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 (capped)", m.Confidence)
	}
}
