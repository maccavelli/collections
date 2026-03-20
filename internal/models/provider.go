// Package models defines data structures and interfaces for
// brainstorming sessions and reasoning providers.
package models

import "context"

// ReasoningProvider defines the interface for an external
// reasoning engine (e.g., an LLM) to participate in the
// brainstorming process.
type ReasoningProvider interface {
	// GenerateQuestions takes a design or proposal and
	// returns targeted Socratic questions or challenges.
	GenerateQuestions(ctx context.Context, design string) ([]string, error)

	// EvaluateQuality provides a structured audit of a
	// design against qualitative benchmarks.
	EvaluateQuality(ctx context.Context, design string) ([]QualityMetric, error)

	// ReviewEvolution analyzes a proposed change for risks.
	ReviewEvolution(ctx context.Context, proposal string) (EvolutionResult, error)
}
