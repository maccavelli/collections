# Design Doc: Brainstorm Clarification Engine (The Decision Fork)

## Overview
The Brainstorm Clarification Engine is a strategic expansion of the `mcp-server-brainstorm` tool. It transforms the server from a passive critic into a proactive "Idea Partner" by identifying technological ambiguity and quantification gaps in user requirements and presenting structured "Decision Forks" (Multiple-Choice Questions) to ground the design.

## Architecture & Data Structures

### 1. The `DecisionFork` Model
A structured response representing an architectural choice the user must make.

```go
type DecisionFork struct {
    Component      string            `json:"component"`      // e.g., "Queue", "Storage", "Auth"
    SocraticPrompt string            `json:"socraticPrompt"` // e.g., "What is the primary requirement for event ordering?"
    Options        map[string]string `json:"options"`        // e.g., {"Strict": "FIFO (Amazon SQS FIFO, Kafka Partition)", "Best-Effort": "Standard SQS"}
    Impact         string            `json:"impact"`         // Why this choice matters (Latency, Cost, Consistency)
    Recommendation string            `json:"recommendation"` // A grounded suggestion for an MVP
}
```

### 2. The `ForkRegistry`
A central, curated registry of common architectural components (e.g., API, Queue, Database, Auth, Cache) mapped to their corresponding "Decision Forks."

## Data Flow & Logic

1.  **Normalization & Scan**: The engine normalizes the input text and scans for "Grounding Triggers" (e.g., *API, Queue, Database*).
2.  **Ambiguity Scoring**: For each triggered component, the engine checks for missing "Non-Functional Modifiers" (e.g., *retention, throughput, consistency*). If a modifier is missing, its corresponding "Fork" is activated.
3.  **Conflict Filtering**: Already-specified constraints (e.g., "GCP only") filter out non-compliant options from the `Options` map.
4.  **Socratic Prompt Generation**: The engine returns a list of `DecisionForks` with clear, easy-to-answer multiple-choice questions.

## Error Handling & Stability

-   **No Match Fallback**: Returns a general "Architectural Grounding Checklist" if no triggers are found.
-   **Saturation Control**: Caps the output at 3 critical forks (prioritizing Security and Availability) to avoid overwhelming the user.
-   **Invalid Input**: Standard MCP error codes for empty or malformed designs.

## Testing Strategy

-   **Unit Tests**: Table-driven tests for keyword detection and fork mapping logic.
-   **Integration Tests**: Verifying the `get_decision_forks` tool response format and interaction with the `engine`.
-   **Edge Cases**: Testing with highly specific vs. highly ambiguous requirements.

## Impact
This feature enables AI agents to help users "ground" their ideas by presenting valid technical paths they might not have considered, reducing unexamined assumptions and architectural debt before implementation begins.
