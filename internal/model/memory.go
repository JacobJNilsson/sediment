// Package model defines the core data types for the sediment memory system.
//
// Sediment uses a geology metaphor: memories are deposited as layers of
// sediment that erode over time, can be compacted under pressure, and
// excavated when needed.
package model

import "time"

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
}

// IsRetrievable reports whether the memory can be surfaced to a query.
func (m *Memory) IsRetrievable() bool {
	return m.State == StateActive || m.State == StateDormant
}
