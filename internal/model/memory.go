// Package model defines the core data types for the sediment memory system.
//
// Sediment uses a geology metaphor: memories are deposited as layers of
// sediment that erode over time, can be compacted under pressure, and
// excavated when needed.
package model

import (
	"fmt"
	"time"
)

// Hardness represents durability on the Mohs scale (1–10).
// Softer memories erode faster; harder memories resist decay.
//
//	1–3  Talc–Calcite:    situational, one-off (high decay rate)
//	4–6  Fluorite–Feldspar: decisions, preferences (moderate decay)
//	7–10 Quartz–Diamond:   conventions, patterns  (low decay rate)
type Hardness int

const (
	HardnessMin     Hardness = 1
	HardnessDefault Hardness = 5
	HardnessMax     Hardness = 10
)

func (h Hardness) Valid() bool {
	return h >= HardnessMin && h <= HardnessMax
}

func (h Hardness) Validate() error {
	if !h.Valid() {
		return fmt.Errorf("hardness %d out of range [%d, %d]", h, HardnessMin, HardnessMax)
	}
	return nil
}

// State represents the lifecycle state of a memory.
type State string

const (
	// StateActive memories are fresh and fully accessible.
	StateActive State = "active"
	// StateDormant memories have decayed below the dormancy threshold.
	StateDormant State = "dormant"
	// StateArchived memories have been compressed/compacted.
	StateArchived State = "archived"
)

// Memory is a single unit of remembered information.
type Memory struct {
	// ID is the unique identifier (UUID).
	ID string `json:"id"`
	// Content is the textual content of the memory.
	Content string `json:"content"`
	// Confidence is the current confidence score [0.0, 1.0].
	Confidence float64 `json:"confidence"`
	// State is the lifecycle state.
	State State `json:"state"`
	// AccessCount tracks how many times this memory has been excavated.
	AccessCount int `json:"access_count"`
	// CreatedAt is when the memory was deposited.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the memory was last modified.
	UpdatedAt time.Time `json:"updated_at"`
	// LastAccessedAt is when the memory was last excavated.
	LastAccessedAt time.Time `json:"last_accessed_at"`
	// Tags are free-form labels for categorisation.
	Tags []string `json:"tags"`
	// Source records where the memory came from (optional).
	Source string `json:"source,omitempty"`
	// Hardness is the durability rating on the Mohs scale (1–10).
	Hardness Hardness `json:"hardness"`
}

// ValidStates is the set of recognised lifecycle states.
var ValidStates = map[State]bool{
	StateActive:   true,
	StateDormant:  true,
	StateArchived: true,
}

// IsRetrievable reports whether the memory can be surfaced to a query.
func (m *Memory) IsRetrievable() bool {
	return m.State == StateActive || m.State == StateDormant
}
