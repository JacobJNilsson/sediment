package model_test

import (
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
)

func TestHardness_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		h    model.Hardness
		want bool
	}{
		{"zero is invalid", 0, false},
		{"min is valid", 1, true},
		{"mid is valid", 5, true},
		{"max is valid", 10, true},
		{"above max is invalid", 11, false},
		{"negative is invalid", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.h.Valid(); got != tt.want {
				t.Errorf("Hardness(%d).Valid() = %v, want %v", tt.h, got, tt.want)
			}
		})
	}
}

func TestHardness_Validate(t *testing.T) {
	t.Parallel()

	if err := model.HardnessDefault.Validate(); err != nil {
		t.Errorf("HardnessDefault.Validate() = %v, want nil", err)
	}
	if err := model.Hardness(0).Validate(); err == nil {
		t.Error("Hardness(0).Validate() = nil, want error")
	}
}

func TestIsRetrievable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state model.State
		want  bool
	}{
		{"active is retrievable", model.StateActive, true},
		{"dormant is retrievable", model.StateDormant, true},
		{"archived is not retrievable", model.StateArchived, false},
		{"unknown state is not retrievable", model.State("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &model.Memory{
				ID:        "test-id",
				Content:   "test content",
				State:     tt.state,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if got := m.IsRetrievable(); got != tt.want {
				t.Errorf("IsRetrievable() = %v, want %v", got, tt.want)
			}
		})
	}
}
