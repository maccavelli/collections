# MCR Server Go Refactor Complete Coverage Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Achieve near-100% test coverage across all packages in `mcp-server-go-refactor`, starting with the lowest coverage areas.

**Architecture:** Use table-driven tests for core logic (formatters, matchers) and functional/integration tests for Handlers/Apply logic. Mock the filesystem or use temporary directories where necessary for transformation tests.

**Tech Stack:** Go 1.26+, `testing` package, `github.com/stretchr/testify/assert`.

---

### Task 1: Coverage for internal/tags/analyzer.go (Part 1 - Core logic)

**Files:**
- Modify: `internal/tags/analyzer_test.go`
- Target: `internal/tags/analyzer.go`

**Step 1: Improve formatCase coverage**
Identify edge cases for `formatCase` (empty string, already formatted, multiple words).

**Step 2: Write tests for formatCase**
Add tests focusing on `snake`, `camel`, and `pascal` conversions.

**Step 3: Run and verify**
Run: `go test -v -cover ./internal/tags`
Expected: Coverage for `formatCase` > 80%.

### Task 2: Coverage for internal/tags/analyzer.go (Part 2 - ApplyTags)

**Files:**
- Modify: `internal/tags/analyzer_test.go`
- Target: `internal/tags/analyzer.go:139`

**Step 1: Mock a struct with tags**
Create a test helper to generate a temporary Go file with a struct.

**Step 2: Implement TestApplyTags**
Call `ApplyTags` on the temporary file and verify the transformation using `dstutil`.

**Step 3: Run and verify**
Run: `go test -v -cover ./internal/tags`
Expected: Coverage for `ApplyTags` > 80%.

### Task 3: Coverage for internal/tags/analyzer.go (Part 3 - Handle/Register)

**Files:**
- Modify: `internal/tags/analyzer_test.go`
- Target: `internal/tags/analyzer.go:36`

**Step 1: Test Register and Metadata**
Verify `Register` correctly populates the tool registry and `Metadata` returns expected values.

**Step 2: Test Handle method**
Mock arguments and verify `Handle` orchestrates `AnalyzeTags` and `ApplyTags` correctly.

**Step 3: Run and verify**
Run: `go test -v -cover ./internal/tags`
Expected: Package coverage > 90%.

### Task 4: Coverage for internal/modernizer/modernizer.go

**Files:**
- Create: `internal/modernizer/modernizer_test.go`
- Target: `internal/modernizer/modernizer.go`

**Step 1: Implement TestHandle**
Simulate a request to the modernizer tool and verify it processes correctly.

**Step 2: Implement TestApplyModernize**
Test the transformation logic for `slices.Filter` replacement and other modernization rules.

**Step 3: Run and verify**
Run: `go test -v -cover ./internal/modernizer`
Expected: Package coverage > 85%.

### Task 5: Coverage for internal/runner/runner.go

**Files:**
- Create: `internal/runner/runner_test.go`
- Target: `internal/runner/runner.go`

**Step 1: Test basic execution loop**
Verify the runner handles tools and error states.

**Step 2: Run and verify**
Run: `go test -v -cover ./internal/runner`
Expected: Package coverage > 85%.

### Task 6: Final Verification

**Step 1: Run full coverage report**
Run: `go test -cover ./...`

**Step 2: Commit all fixes**
GIT_EDITOR=true git commit
