# Brainstorm MCP Tool Manual Testing Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Verify all 8 `brainstorm` MCP tools and measure their execution speed against the project's own source code.

**Architecture:** Sequential execution of MCP tool calls with precise start/end timing using `date +%s%3N` in the terminal to capture millisecond-level resolution where possible, or relying on tool call metadata. Results will be summarized in a final report.

**Tech Stack:** MCP, Bash, Go (server).

---

### Task 1: Environment Setup
**Files:**
- Create: `/tmp/brainstorm_test_setup.sh`

**Step 1: Verify server is running and tools are available**
Run: `mcp_brainstorm_get_internal_logs`
Expected: Returns empty or recent logs.

**Step 2: Commit**
`GIT_EDITOR=true git commit` (Skip if no changes to repo files)

### Task 2: Execute Discovery Tools
**Step 1: Run `analyze_project`**
Run: `mcp_brainstorm_analyze_project(path="/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-brainstorm/")`
Note: Record start/end time.

**Step 2: Run `suggest_next_step`**
Run: `mcp_brainstorm_suggest_next_step(path="/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-brainstorm/")`
Note: Record start/end time.

### Task 3: Execute Socratic Reasoning Tools
**Step 1: Run `challenge_assumption`**
Run: `mcp_brainstorm_challenge_assumption(design="Use a persistent SQLite database for session storage.")`
Note: Record start/end time.

**Step 2: Run `red_team_review`**
Run: `mcp_brainstorm_red_team_review(design="Expose the brainstorming tools via a public REST API.")`
Note: Record start/end time.

**Step 3: Run `evaluate_quality_attributes`**
Run: `mcp_brainstorm_evaluate_quality_attributes(design="Implement a layered architecture with independent modules and a Redis caching layer for performance.")`
Note: Record start/end time.

### Task 4: Execute Evolution and Decision Tools
**Step 1: Run `analyze_evolution`**
Run: `mcp_brainstorm_analyze_evolution(proposal="Refactor the internal/engine module to use an LLM provider instead of heuristic logic.")`
Note: Record start/end time.

**Step 2: Run `capture_decision_logic`**
Run: `mcp_brainstorm_capture_decision_logic(decision="Use Go for the server implementation for concurrency and safety.", alternatives="Python, Node.js")`
Note: Record start/end time.

### Task 5: Finalize and Report
**Step 1: Run `get_internal_logs`**
Run: `mcp_brainstorm_get_internal_logs()`
Note: Capture any runtime errors or warnings.

**Step 2: Compile Report**
Create: `docs/reports/2026-03-19-brainstorm-test-report.md`
Summary of all durations and output qualities.

**Step 3: Final Commit**
`GIT_EDITOR=true git commit`
