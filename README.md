# 🛠️ Go Refactor MCP Server

A comprehensive, production-grade Model Context Protocol (MCP) server for advanced Go source code refactoring,
static analysis, and structural optimization.

## 🚀 Overview

The Go Refactor server provides deep structural analysis and automated transformation tools designed to bring legacy
Go codebases into alignment with modern standards. It moves beyond simple syntax checking to provide architectural
insights, safety audits, and performance tuning.

### 📋 Core Pillars

1. **Structural Analysis**: Deep inspection of interfaces, call graphs, context propagation, and cyclic
   dependencies.
2. **Resource Optimization**: Automated struct alignment analysis to reduce heap usage and improve performance.
3. **Modernization**: Intelligent conversion of legacy patterns to modern Go idiomatics (Go 1.21+).
4. **Safety & Security**: Active detection of dynamic SQL string construction and other vulnerable patterns.

---

## 🛠️ Tools (The 14-Stage Master Pipeline)

The server follows a mandatory sequence for comprehensive project refactoring:

### Phase 1: Preparation

- `go_complexity_analyzer`: Analyze project size and "God functions".
- `go_dead_code_pruner`: Clean up dead code and unreferenced variables.
- `go_package_cycler`: Check for cyclic dependencies blocking compilation.

### Phase 2: Structural Refactoring

- `go_interface_discovery`: Identify shared architectural patterns.
- `find_interface_implementations`: Map abstractions and refactoring blast radius.
- `go_interface_tool`: Extract interfaces and finalize architecture.

### Phase 3: Reliability & Performance

- `go_context_analyzer`: Ensure correct context propagation.
- `go_struct_layout`: Reorder struct fields for optimal memory alignment.
- `go_tag_manager`: Standardize JSON/YAML struct tags.
- `go_modernizer`: Upgrade to optimized standard library features.

### Phase 4: Final Audits

- `go_sql_injection_guard`: Perform dynamic SQL vulnerability scanning.
- `go_doc_generator`: Add or fix required godocs.
- `go_dependency_impact`: Map transitive impact of new dependencies.
- `go_test_coverage_tracer`: Run tests and verify the entire refactor.

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-go-refactor main.go
```

### 2. Configure for IDEs

#### **Antigravity**

```yaml
mcpServers:
  go-refactor:
    command: "/absolute/path/to/dist/mcp-server-go-refactor"
```

#### **VS Code (MCP Extension / Cline)**

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/absolute/path/to/dist/mcp-server-go-refactor",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

#### **Cursor IDE**

1. **Settings** -> **Features** -> **MCP**.
2. **+ Add New MCP Server**.
3. Name: `Go Refactor`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-go-refactor`

---

## 📖 Use Cases

- **Legacy Modernization**: Use `go_modernizer` to revitalize older internal libraries.
- **CI/CD Guardrails**: Integrate `go_sql_injection_guard` into automated quality gates.
- **Performance Tuning**: Minimize memory overhead using `go_struct_layout`.

---

## 💻 CLI Functionality

Check version:

```bash
./mcp-server-go-refactor -version
```

---

*Built with Go for professional refactoring at scale.*
