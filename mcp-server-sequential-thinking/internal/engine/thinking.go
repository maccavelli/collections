package engine

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"mcp-server-sequential-thinking/internal/util"
)

// ThoughtData represents a single structured thinking step for the sequential thought process.
type ThoughtData struct {
	util.UniversalBaseInput
	Thought               string  `json:"thought" jsonschema:"[REQUIRED] Your current thinking step"`
	SelfCritique          string  `json:"selfCritique" jsonschema:"[REQUIRED] Explicit self-correction and rigorous evaluation of the current thought logic"`
	ContradictionDetected bool    `json:"contradictionDetected" jsonschema:"[REQUIRED] Must be set to true if SelfCritique identifies a paradox, flaw, or assumption"`
	ResolutionStrategy    *string `json:"resolutionStrategy,omitempty" jsonschema:"Mandatory step outlining how to fix the logic ONLY if the previous thought had a contradiction"`
	NextThoughtNeeded     bool    `json:"nextThoughtNeeded" jsonschema:"[REQUIRED] Whether another thought step is needed"`
	ThoughtNumber     int     `json:"thoughtNumber" jsonschema:"[REQUIRED] Current thought number (numeric value, e.g., 1, 2, 3)" jsonschema_extras:"minimum=1"`
	TotalThoughts     int     `json:"totalThoughts" jsonschema:"[REQUIRED] Estimated total thoughts needed (numeric value, e.g., 5, 10)" jsonschema_extras:"minimum=1"`
	IsRevision        *bool   `json:"isRevision,omitempty" jsonschema:"Whether this revises previous thinking"`
	RevisesThought    *int    `json:"revisesThought,omitempty" jsonschema:"Which thought is being reconsidered" jsonschema_extras:"minimum=1"`
	BranchFromThought *int    `json:"branchFromThought,omitempty" jsonschema:"Branching point thought number" jsonschema_extras:"minimum=1"`
	BranchID          *string `json:"branchId,omitempty" jsonschema:"Branch identifier"`
	NeedsMoreThoughts *bool   `json:"needsMoreThoughts,omitempty" jsonschema:"If more thoughts are needed"`
	ActionableInsight *bool   `json:"actionableInsight,omitempty" jsonschema:"If true, marks the current thought as containing an actionable conclusion or final insight"`
}

// OutputData represents the final structured output after processing a thought.
type OutputData struct {
	ThoughtNumber        int      `json:"thoughtNumber"`
	TotalThoughts        int      `json:"totalThoughts"`
	NextThoughtNeeded    bool     `json:"nextThoughtNeeded"`
	Branches             []string `json:"branches"`
	ThoughtHistoryLength int      `json:"thoughtHistoryLength"`
}

// Engine provides state management for sequential thinking steps and branches.
type Engine struct {
	mu                    sync.Mutex
	thoughtHistory        []ThoughtData
	branches              map[string][]ThoughtData
	disableThoughtLogging bool
}

// NewEngine creates a new Engine instance initialized with environment configurations.
func NewEngine() *Engine {
	disableThoughtLogging := strings.ToLower(os.Getenv("DISABLE_THOUGHT_LOGGING")) == "true"
	return &Engine{
		thoughtHistory:        make([]ThoughtData, 0, 100),
		branches:              make(map[string][]ThoughtData),
		disableThoughtLogging: disableThoughtLogging,
	}
}

func (e *Engine) formatThought(input ThoughtData) string {
	prefix := ""
	contextStr := ""

	if input.IsRevision != nil && *input.IsRevision && input.RevisesThought != nil {
		prefix = "🔄 Revision"
		contextStr = fmt.Sprintf(" (revising thought %d)", *input.RevisesThought)
	} else if input.BranchFromThought != nil && input.BranchID != nil {
		prefix = "🌿 Branch"
		contextStr = fmt.Sprintf(" (from thought %d, ID: %s)", *input.BranchFromThought, *input.BranchID)
	} else if input.ContradictionDetected {
		prefix = "⚠️ Contradiction"
	} else if input.ResolutionStrategy != nil && *input.ResolutionStrategy != "" {
		prefix = "💡 Resolution"
	} else {
		prefix = "💭 Thought"
	}

	return fmt.Sprintf("[%s %d/%d%s] %s", prefix, input.ThoughtNumber, input.TotalThoughts, contextStr, input.Thought)
}

// ProcessThought records a single thought step, evaluates branching, and determines the next step.
func (e *Engine) ProcessThought(input ThoughtData) (*OutputData, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 🛡️ Continuous Dialectic Gatekeeper: Forcing Socratic Evolution
	var previousContradiction bool
	if len(e.thoughtHistory) > 0 {
		lastThought := e.thoughtHistory[len(e.thoughtHistory)-1]
		if lastThought.ContradictionDetected {
			previousContradiction = true
		}
	}

	// Rule 1: A prior contradiction MUST be addressed sequentially organically.
	if previousContradiction {
		if input.ResolutionStrategy == nil || *input.ResolutionStrategy == "" {
			return nil, fmt.Errorf("CONTINUOUS DIALECTIC VIOLATION: The previous thought encountered a contradiction. You MUST supply a 'resolutionStrategy' in this thought to proceed")
		}
	}

	// Rule 2: A sequence CANNOT conclude while a contradiction is actively open mathematically.
	if input.ContradictionDetected && !input.NextThoughtNeeded {
		return nil, fmt.Errorf("CONTINUOUS DIALECTIC VIOLATION: You cannot set 'nextThoughtNeeded=false' while 'contradictionDetected' is true. You must dynamically generate another thought executing the resolution strategy")
	}

	if input.ThoughtNumber > input.TotalThoughts {
		input.TotalThoughts = input.ThoughtNumber
	}

	e.thoughtHistory = append(e.thoughtHistory, input)

	if input.BranchFromThought != nil && input.BranchID != nil {
		branchID := *input.BranchID
		if _, exists := e.branches[branchID]; !exists {
			e.branches[branchID] = []ThoughtData{}
		}
		e.branches[branchID] = append(e.branches[branchID], input)
	}

	if !e.disableThoughtLogging {
		slog.Info(e.formatThought(input), "number", input.ThoughtNumber, "total", input.TotalThoughts)
	}

	branchesKeys := make([]string, 0, len(e.branches))
	for k := range e.branches {
		branchesKeys = append(branchesKeys, k)
	}

	return &OutputData{
		ThoughtNumber:        input.ThoughtNumber,
		TotalThoughts:        input.TotalThoughts,
		NextThoughtNeeded:    input.NextThoughtNeeded,
		Branches:             branchesKeys,
		ThoughtHistoryLength: len(e.thoughtHistory),
	}, nil
}
