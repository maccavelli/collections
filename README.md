# Go Refactor Server

A comprehensive, production-grade MCP server for advanced Go source code refactoring, static analysis, and structural optimization.

## Overview

The Go Refactor server provides deep structural analysis and automated transformation tools designed to bring legacy Go codebases into alignment with modern standards. It moves beyond simple syntax checking to provide architectural insights, safety audits, and performance tuning for high-throughput applications.

### What it does (Core Pillars)

1.  **Structural Analysis**: Deep inspection of interfaces, call graphs, context propagation, and cyclic dependencies.
2.  **Resource Optimization**: Automated struct alignment and memory padding analysis to reduce heap usage and improve performance.
3.  **Modernization**: Intelligent conversion of legacy patterns to modern Go idiomatics (e.g., `slices` and `maps` packages in 1.21+).
4.  **Safety & Security**: Active detection of dynamic SQL string construction and other vulnerable patterns.
5.  **Refactoring Automation**: Large-scale tag standardization, dead code pruning, and documentation audits.

### How it works (Architecture)

Built using the latest MCP SDK and Go 1.26.1+, the server utilizes an explicit registration-loader architecture for high reliability:

-   **Explicit Tool Registry**: Eschews side-effect initialization in favor of intentional tool registration, allowing for better observability and safer dependency injection.
-   **AST-Driven Intelligence**: Uses standard library `go/ast`, `go/types`, and `go/parser` to ensure absolute accuracy in source code manipulation.
-   **Embedded Multi-Diagnostic System**: Real-time internal logging is accessible via a dedicated `LogBuffer` and the `go-refactor://logs` resource.
-   **Parallel Analysis Engine**: Implements high-concurrency structural discovery using `errgroup` and `sync.Mutex`, enabling rapid interface clustering and cross-package analysis even in massive codebases.
-   **Standardized Handler Interface**: All tools implement a unified interface, promoting consistent behavior and error handling across the entire suite.

### Why it exists (Rationale)

Modern Go development requires more than just formatting. Large codebases often suffer from "structural rot"—inefficient memory layouts, hidden cyclic imports, inconsistent interface patterns, and dropped contexts. The Go Refactor server empowers AI-driven agents and developers to:

-   **Enforce Alignment**: Standardize tags and documentation across thousands of files instantly.
-   **Optimize for Performance**: Identify and fix wasted memory padding in critical data structures.
-   **Modernize Legacy Code**: Automatically upgrade manual loops to optimized standard library functions.
-   **Guard the Perimeter**: Maintain security by preventing the introduction of known anti-patterns.

## Tools & Capabilities

### Code Analysis & Optimization
-   `go_complexity_analyzer(pkg)`: Calculates cyclomatic and cognitive complexity for all functions in a package to identify "God functions."
-   `go_struct_alignment_optimizer(pkg, structName)`: Detects wasted padding in structs and recommends optimal field ordering for memory efficiency.
-   `go_package_cycler(pkg)`: Performs a comprehensive scan to detect and visualize cyclic import paths in the module.
-   `go_context_analyzer(pkg)`: Audits call chains to ensure robust context propagation and identify dropped signals (e.g., using `context.TODO()` in production).

### Refactoring & Transformation
-   `go_interface_tool(pkg, structName, [ifaceName])`: Analyzes interface compatibility or extracts new interface definitions from existing structs.
-   `go_interface_discovery(pkg)`: Analyzes structural signatures to discover hidden abstractions and shared method patterns across a package.
-   `go_tag_manager(pkg, structName, targetTag, caseFormat)`: Standardizes and transforms field tags (e.g., JSON, YAML) across a structure.
-   `go_dead_code_pruner(pkg)`: Identifies unreferenced package-level functions, variables, and constants to reduce maintenance surface.
-   `go_modernizer(pkg, [rewrite])`: Scans for legacy patterns that can be replaced by optimized standard library functions (Go 1.21+ slices/maps).

### Quality & Safety
-   `go_sql_injection_guard(pkg)`: Scans for dynamic SQL string construction vulnerabilities.
-   `go_doc_generator(pkg)`: Audits exported identifiers lacking documentation and generates standard Go commentary skeletons.
-   `go_dependency_impact(pkg)`: Evaluates the "blast radius" of updating external dependencies by mapping their transitive influence.
-   `go_test_coverage_tracer(pkg)`: Runs tests and provides a condensed, failure-focused coverage report with actionable logs.

### System Diagnostics
-   `get_internal_logs(max_lines)`: Retrieves the most recent internal server logs from the log buffer for transparency and debugging.

## Installation

### 1. Build the Binary

Ensure you have Go installed, then build the binary:

```bash
go build -o dist/mcp-server-go-refactor main.go
```

The compiled binary will be located in the `dist` directory.

### 2. Configuration for AI Agents (Antigravity, Claude, Cline)

To use this server with an MCP-compatible agent, add it to your `mcpServers` configuration file.

#### **Windows**
> [!IMPORTANT]
> On Windows, you **MUST** include the `.exe` extension in the command path for the agent to correctly invoke the binary.

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "C:\\path\\to\\mcp-server-go-refactor.exe",
      "args": [],
      "env": {
        "PATH": "C:\\Program Files\\Go\\bin;C:\\Windows\\system32"
      }
    }
  }
}
```

#### **Linux / MacOS**
```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/usr/local/bin/mcp-server-go-refactor",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

## Use Cases

- **Legacy Modernization**: Use `go_modernizer` and `go_tag_manager` to quickly revitalize older internal libraries.
- **CI/CD Guardrails**: Integrate `go_sql_injection_guard` and `go_package_cycler` into pipelines for automated quality gates.
- **Performance Tuning**: Identify high-throughput structs using `go_struct_alignment_optimizer` to minimize memory overhead.
- **Audit & Discovery**: Use `go_context_analyzer` and `go_interface_discovery` to understand complex inheritance and signal flows in large projects.

---

Built with Go for performance and reliability.
