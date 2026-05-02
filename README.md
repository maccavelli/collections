# 🐹 MagicGo-Refactor Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for automated Go codebase analysis, refactoring, and standards enforcement.

## 🚀 Overview

`mcp-server-go-refactor` is the primary intelligence engine for Go development in the MagicTools suite. It understands Go AST (Abstract Syntax Tree), complexity metrics, and idiomatic patterns to provide automated code transformations.

### 📋 Core Pillars

1.  **AST-Safe Mutations**: Applies code changes using structured AST transformations instead of brittle regex.
2.  **Complexity Analysis**: Identifies deeply nested logic, long functions, and cognitive debt.
3.  **Modernization**: Automatically upgrades code to use Go 1.26.2+ idioms.
4.  **Integrated Toolchain**: Provisions its own isolated Go environment to ensure consistent results across platforms.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`analyze_complexity`**: Scans a package and identifies functions exceeding complexity thresholds.
*   **`suggest_fixes`**: Generates idiomatic Go 1.26.2+ refactoring suggestions for a specific file.
*   **`apply_vetted_edit`**: Safely applies a structural code change using AST-aware logic.
*   **`generate_implementation_plan`**: (Internal) Used by the orchestrator to plan refactoring DAGs.

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

## ⚙️ Configuration

### 1. Build the Binary
```bash
make build
```

### 2. Standalone Environment Variables
| Variable | Description |
| :--- | :--- |
| `MCP_API_URL` | Upstream context servers (e.g., `recall`). |
| `GOMEMLIMIT` | Resource boundary (default: 1024MiB). |

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/absolute/path/to/mcp-server-go-refactor"
    }
  }
}
```

### 💻 VSCode (Roo Code / Cline)
**Paths:**
*   **Linux/macOS**: `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json`
*   **Windows**: `%APPDATA%\Code\User\globalStorage\rooveterinaryinc.roo-cline\settings\cline_mcp_settings.json`

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "C:/path/to/mcp-server-go-refactor.exe"
    }
  }
}
```

### 🤖 Claude Desktop
**Paths:**
*   **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
*   **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "go-refactor": {
      "command": "/usr/local/bin/mcp-server-go-refactor"
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
