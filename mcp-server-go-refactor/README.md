# Go Refactor Server

A comprehensive, production-grade MCP server for advanced Go source code refactoring, static analysis, and structural optimization.

## Overview

The Go Refactor server provides deep structural analysis and automated transformation tools designed to bring legacy Go codebases into alignment with modern "premium" standards. It transitions beyond simple syntax checking to provide meaningful architectural insights and safety audits.

### What it does (Core Pillars)

1.  **Structural Analysis**: Deep inspection of interfaces, call graphs, and cyclic dependencies.
2.  **Resource Optimization**: Automated struct alignment and memory padding analysis to reduce heap usage and improve performance.
3.  **Safety & Security**: Active detection of dynamic SQL string construction and other vulnerable patterns.
4.  **Refactoring Automation**: Large-scale tag standardization, dead code pruning, and documentation generation.

### How it works (Architecture)

Built using the latest MCP SDK and Go 1.26.1+, the server utilizes an explicit registration-loader architecture for high reliability:

-   **Explicit Tool Registry**: Eschews side-effect initialization in favor of intentional tool registration, allowing for better observability and safer dependency injection.
-   **AST-Driven Intelligence**: Uses standard library `go/ast`, `go/types`, and `go/parser` to ensure absolute accuracy in source code manipulation.
-   **Embedded Multi-Diagnostic System**: Real-time internal logging is accessible via a dedicated `LogBuffer` and the `go-refactor://logs` resource.
-   **Standardized Handler Interface**: All tools implement a unified interface, promoting consistent behavior and error handling across the entire suite.

### Why it exists (Rationale)

Modern Go development requires more than just formatting. Large codebases often suffer from "structural rot"—inefficient memory layouts, hidden cyclic imports, and inconsistent interface patterns. The Go Refactor server empowers AI-driven agents and developers to:

-   **Enforce Alignment**: Standardize tags and documentation across thousands of files instantly.
-   **Optimize for Performance**: Identify and fix wasted memory padding in critical data structures.
-   **Guard the Perimeter**: Maintain security by preventing the introduction of known anti-patterns.

## Tools & Capabilities

### Code Analysis & Optimization
-   `go_complexity_analyzer(pkg)`: Calculates cyclomatic complexity for all functions in a package to identify high-risk areas.
-   `go_struct_alignment_optimizer(pkg, structName)`: Detects wasted padding in structs and recommends optimal field ordering for memory efficiency.
-   `go_package_cycler(pkg)`: Performs a comprehensive scan to detect and visualize cyclic import paths in the module.

### Refactoring & Transformation
-   `go_interface_tool(pkg, structName, [ifaceName])`: Analyzes interface compatibility or extracts new interface definitions from existing structs.
-   `go_tag_manager(pkg, structName, targetTag, caseFormat)`: Standardizes and transforms field tags (e.g., JSON, YAML) across a structure.
-   `go_dead_code_pruner(pkg)`: Identifies unused exported or internal functions and variables to reduce binary size and complexity.

### Quality & Safety
-   `go_sql_injection_guard(pkg)`: Scans for dynamic SQL string construction vulnerabilities.
-   `go_doc_generator(pkg)`: Audits and generates documentation skeletons for exported identifiers lacking commentary.
-   `go_test_coverage_tracer(pkg)`: Runs tests and provides a condensed, failure-focused coverage report.

### System Diagnostics
-   `get_internal_logs(max_lines)`: Retrieves the most recent internal server logs from the log buffer for transparency and debugging.

## Installation

### 1. Build the Binary
```bash
make build
```
The compiled binary will be located in the `dist` directory.

### 2. Configure for IDEs

#### **Antigravity**
Add the server to your `mcpServers` configuration:
```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/usr/local/bin/mcp-server-go-refactor",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin"
      }
    }
  }
}
```

#### **VS Code (MCP Extension)**
If using an MCP-compatible VS Code extension (like the Claude Dev or Cline):
1.  Navigate to the setting/config file for the extension.
2.  Add the configuration entry:
```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/path/to/dist/mcp-server-go-refactor",
      "args": []
    }
  }
}
```

## Use Cases

-   **Legacy Modernization**: Use `go_tag_manager` and `go_doc_generator` to quickly revitalize older internal libraries.
-   **CI/CD Guardrails**: Integrate `go_sql_injection_guard` and `go_package_cycler` into pipelines for automated quality gates.
-   **Performance Tuning**: Identify high-throughput structs using `go_struct_alignment_optimizer` to minimize memory overhead.

---

Built with Go for performance and reliability.
