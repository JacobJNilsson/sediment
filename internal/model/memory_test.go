package model_test

import (
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
)

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
