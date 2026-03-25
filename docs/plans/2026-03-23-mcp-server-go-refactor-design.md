# MCP Server Go Refactor - Design Document

## 1. Overview
The `mcp-server-go-refactor` is an MCP (Model Context Protocol) server designed to give AI agents deep, systematic, and correct refactoring and analysis tools for Go projects. 

Instead of relying on regex or raw text dumps, this server provides structured analysis of the Go Abstract Syntax Tree (AST), package dependencies, and test coverage JSON streams to offload rote parsing tasks from the LLM, leaving only distilled, actionable JSON data.

## 2. Architecture: Domain-Specific Service Modules
We will adopt **Approach A** (Component-Based Services), mirroring the best practices established in the workspace (e.g., `mcp-server-magicskills`). 

### Core Components:
- **`cmd/server`**: The entry point. Handles initial configuration, logging, graceful shutdown, and server instantiation.
- **`internal/handler`**: The router. Intercepts incoming `mcp.CallToolRequest`, validates arguments, and delegates to specific service packages.
- **Service Modules** (The true engines of the server):
  - **`internal/dependency`**: Handles `go list` JSON parsing and module graph analysis.
  - **`internal/astutil`**: Leverages `golang.org/x/tools/go/packages` and `go/ast` for type-safe static analysis.
  - **`internal/coverage`**: Manages test execution and parsing of `go test -json` traces.
  - **`internal/graph`**: Manages call graphs and import cycles.

## 3. Tool Specifications (MVP Set)

1. **`go_dependency_impact`**: Runs `go list -m -u -json <pkg>` and detects if adding an external module causes transitive breaking changes (version bumps).
2. **`go_interface_analyzer`**: Uses AST to compare a struct's actual methods against a predefined interface to explicitly check compatibility and locate missing signatures.
3. **`go_test_coverage_tracer`**: Executes a targeted test run, parses the raw JSON stream, and returns a condensed map of exact failing tests, packages, and the immediate preceding error log.
4. **`go_package_cycler`**: Traverses project import chains to detect and isolate cyclic imports, returning the exact shortest path causing the loop.
5. **`go_call_graph_analyzer`**: Analyzes the blast radius of a function change by providing a strict list of all its transitive callers.
6. **`go_struct_alignment_optimizer`**: Uses `go/types.Sizes` to detect wasted padding in structs and recommends the exact field ordering to reduce memory usage.
7. **`go_tag_manager`**: AST-based transformation that restructures or standardizes field tags on a struct across an entire file.

## 4. Data Flow
1. **Request Phase**: The LLM calls a tool via the MCP protocol.
2. **Routing Phase**: The `handler` module validates the inputs (e.g., absolute path to a Go file, struct name).
3. **Execution Phase**: The target service module (e.g., `astutil`) loads the module context, parses the AST or executes the OS-level standard tooling.
4. **Synthesis Phase**: The raw output (which might be gigabytes of trace data or thousands of lines of terminal output) is filtered down to exact, relevant JSON structs.
5. **Response Phase**: Data is wrapped in an `mcp.CallToolResult` and piped back to the LLM for immediate, context-efficient reasoning.

## 5. Error Handling & Stability Constraints
- **Sub-Process Timeouts**: All `exec.CommandContext` calls must enforce strict timeouts (e.g., 60s) to prevent frozen compiler daemons.
- **Panic Boundaries**: AST parsing can occasionally panic on profoundly malformed code. Each executing tool handler must feature recovery middleware.
- **Memory Boundaries**: `go test -json` streams will be extremely dense. We must stream parser results rather than buffering the entire output string in memory to prevent OOM errors.
