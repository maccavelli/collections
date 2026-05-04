# 🐹 MagicGo-Refactor Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for automated Go codebase
analysis, refactoring, and standards enforcement.

## 🚀 Overview

`mcp-server-go-refactor` is the primary intelligence engine for Go development in the MagicTools
suite. It understands Go AST (Abstract Syntax Tree), complexity metrics, and idiomatic patterns to
provide automated code transformations.

### 📋 Core Pillars

* **AST-Safe Mutations**: Applies code changes using structured AST transformations instead of brittle regex.
* **Complexity Analysis**: Identifies deeply nested logic, long functions, and cognitive debt.
* **Modernization**: Automatically upgrades code to use Go 1.26.2+ idioms.
* **Integrated Toolchain**: Provisions its own isolated Go environment to ensure consistent results across platforms.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`analyze_complexity`**: Scans a package and identifies functions exceeding complexity thresholds.
* **`suggest_fixes`**: Generates idiomatic Go 1.26.2+ refactoring suggestions for a specific file.
* **`apply_vetted_edit`**: Safely applies a structural code change using AST-aware logic.
* **`generate_implementation_plan`**: (Internal) Used by the orchestrator to plan refactoring DAGs.

### Orchestration with MagicTools (Recommended)

This server is designed to be the primary worker for `magictools:execute_pipeline`.

```json
{
  "name": "magictools:execute_pipeline",
  "arguments": {
    "intent": "refactor the cmd package for better testability",
    "target": "/home/user/project"
  }
}
```

---

## 📦 Installation & Setup

### 1. Build the Binary

```bash
make build
```

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run Go-Refactor as an orchestrated sub-server:

```yaml
- name: go-refactor
  command: /absolute/path/to/mcp-server-go-refactor
  env:
    HOME: /absolute/path/to/home
    MCP_API_URL: http://localhost:7000/mcp # URL for Recall context
    PATH: /usr/local/go/bin:/usr/local/bin:/usr/bin # Ensure Go is in PATH
  memory_limit_mb: 6144
  gomemlimit_mb: 2048
  max_cpu_limit: 2
  disabled: false
  deferred_boot: false
```

### 3. Direct IDE Configuration

If you prefer to run the server standalone, use the following configuration for your IDE:

#### 🌌 Antigravity / VSCode (Roo Code / Cline)

**Paths:**

* **Linux/macOS:** `~/.gemini/antigravity/mcp_config.json`
* **Windows:** `%APPDATA%\Antigravity\mcp_config.json`

##### Linux/macOS Example

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/absolute/path/to/mcp-server-go-refactor",
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp",
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

##### Windows Example (Antigravity Suite)

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "C:\\path\\to\\mcp-server-go-refactor.exe",
      "env": {
        "HOME": "C:\\Users\\YourName",
        "MCP_API_URL": "http://localhost:7000/mcp",
        "PATH": "C:\\Program Files\\Go\\bin;C:\\Windows\\system32"
      }
    }
  }
}
```

#### 🤖 Claude Desktop

**Paths:**

* **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
* **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

##### macOS Example

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/absolute/path/to/mcp-server-go-refactor",
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp",
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

##### Windows Example (Claude Suite)

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "C:\\path\\to\\mcp-server-go-refactor.exe",
      "env": {
        "HOME": "C:\\Users\\YourName",
        "MCP_API_URL": "http://localhost:7000/mcp",
        "PATH": "C:\\Program Files\\Go\\bin;C:\\Windows\\system32"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
