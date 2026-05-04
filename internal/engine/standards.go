// Package engine provides functionality for the engine subsystem.
package engine

// DefaultStandards contains the hardcoded Tier 3 fallback rules for the go-refactor
// toolsuite when running natively in standalone environments without the
// orchestrator's remote recall cluster access.
var DefaultStandards = map[string]string{
	"synthesis_standards": "Consolidate diagnostic metrics into actionable, hygienic patches emphasizing idiomatic Go 1.26.1 performance and readability.",
	"interface_ast":       "Interfaces should be highly encapsulated (io.Reader paradigm). Rely on implicit satisfaction and runtime behavior matching instead of upfront monolithic declaration.",
	"modernizer_ast":      "Struct types should rigidly encapsulate scope. Favor native errors.Is evaluation and robust synchronization paradigms (errgroup).",
	"context_propagation": "Force `context.Context` tracking down entire un-bounded execution paths. Forbid random `context.TODO` boundaries. Respect global cancellation streams.",
	"security_standards":  "Ban reflective unsafe injections. Secure template rendering natively. Sanitize exec boundaries. (gosec compliance).",
	"tag_conventions":     "Struct tags must strictly conform to JSON, YAML, and BSON edge constraints including 'omitempty' mapping paradigms.",
	"metrics_complexity":  "Keep Cyclomatic Complexity under 15 where possible. Cognitive complexity over 20 mandates extraction algorithms.",
	"struct_layout":       "Order struct layouts natively by descending byte-alignment sizes to minimize alignment padding loss in memory.",
	"pruner_standards":    "Scavenge completely un-called package global functions, floating variables, and undocumented dead-paths to reduce the active compilation binary.",
	"godoc_standards":     "Package overviews should concisely explain internal behavior, exported types mandate strict doc-blocks, and logic should clarify the 'why' rather than the 'how'.",
	"cycle_architecture":  "Strictly ban circular import loops. Decouple cross-package cycles strictly using an internal boundary API or interface inversion.",
	"dependency_mgmt":     "Rely natively on Go standard library modules. Prevent module proliferation. Deprecate legacy dependencies and maintain clean go.mod sums.",
	"testing_standards":   "Deploy robust table-driven testing loops (t.Run) mapping exhaustive boundary cases instead of strictly testing happy-paths.",
}
