# Design: Brainstorm MCP Tool Manual Testing

## Goal
Verify the functionality and measure the execution performance (speed) of all 8 tools provided by the `brainstorm` MCP server.

## Success Criteria
- Each tool returns valid, contextually relevant outputs (heuristic or AST-based).
- Precise execution times (ms) are captured for each tool call.
- A final report summarizes the findings.

## Test Target
Project Root: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-brainstorm/`

## Testing Suite
The following sequence will be executed:

1.  **`analyze_project`**: 
    - Input: `path="/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-brainstorm/"`
2.  **`suggest_next_step`**: 
    - Input: `path="/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-brainstorm/"`
3.  **`challenge_assumption`**: 
    - Input: `design="Use a persistent SQLite database for session storage."`
4.  **`red_team_review`**: 
    - Input: `design="Expose the brainstorming tools via a public REST API."`
5.  **`evaluate_quality_attributes`**: 
    - Input: `design="Implement a layered architecture with independent modules and a Redis caching layer for performance."`
6.  **`analyze_evolution`**: 
    - Input: `proposal="Refactor the internal/engine module to use an LLM provider instead of heuristic logic."`
7.  **`capture_decision_logic`**: 
    - Input: `decision="Use Go for the server implementation for concurrency and safety.", alternatives="Python, Node.js"`
8.  **`get_internal_logs`**: 
    - Input: None

## Execution Methodology
1. Record start timestamp.
2. Execute tool call via Antigravity.
3. Record end timestamp on finish.
4. Calculate duration ($Duration = End - Start$).
5. Document summary of result JSON/Text.

## Reporting
The results will be compiled into a report artifact titled `braistorm_testing_report.md`.
