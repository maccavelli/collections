# 🪄 MagicTools Orchestrator

The central backbone of the Antigravity ecosystem. A robust, fault-tolerant Model Context Protocol (MCP) orchestrator, process manager, and gateway.

## 🚀 Overview

`mcp-server-magictools` is the **Master Architect** of the MCP swarm. It is designed to be the **only** MCP server configured directly in your IDE (Antigravity, VSCode, etc.). It acts as a primary proxy, watchdog, and process manager that hot-loads and manages all other sub-servers (e.g., `go-refactor`, `brainstorm`, `recall`) based on its own `servers.yaml` configuration.

### 📋 Core Pillars

1.  **Unified Orchestration**: Manages the lifecycle of multiple MCP sub-servers, providing a single point of entry for the LLM.
2.  **Socratic DAG Generation**: Dynamically composes complex execution pipelines (Directed Acyclic Graphs) for tasks like code refactoring and brainstorming.
3.  **Intelligent Proxying**: Routes tool calls to the appropriate sub-server via the `call_proxy` interface.
4.  **Health & Telemetry**: Monitor the status, latency, and logs of the entire ecosystem from a single dashboard.
5.  **Safety & Security**: Enforces strict resource limits and provides a central point for credential management.

---

## 🛠️ Usage & Functionality

MagicTools is not just another tool; it's the **operating system** for your AI agents. It provides the following top-level capabilities:

*   **`align_tools`**: Resolves high-level intents to specific tool URNs across the entire ecosystem.
*   **`execute_pipeline`**: Launches autonomous workflows that chain multiple tools together with Socratic review gates.
*   **`sync_ecosystem`**: Refreshes all sub-server indices to ensure the orchestrator is always aware of the latest capabilities.
*   **`call_proxy`**: Safely executes tools on sub-servers with built-in error handling and telemetry.

---

## ⚙️ Initial Configuration

Before using MagicTools in your IDE, you must perform the initial setup using the CLI wizard.

### 1. Build the Binary
```bash
make build
```

### 2. Run the Configuration Wizard
The `configure` command is used to set up your LLM providers for semantic search and vector embeddings.
```bash
./dist/mcp-server-magictools configure
```
**What this does:**
*   **Standard Search**: Configures the primary LLM (Gemini, Anthropic, or OpenAI) used for tool alignment and reasoning.
*   **Vector Search**: Sets up the embedding engine (Gemini, Voyage, OpenAI, or Ollama) used for indexing standard libraries and project knowledge.
*   **Persistence**: Automatically creates and updates `~/.config/mcp-server-magictools/config.yaml`.

---

## 🖥️ IDE Configuration Examples

MagicTools should be the **only** MCP server configured in your IDE to enable the full orchestration suite.

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "magictools": {
      "command": "/absolute/path/to/scripts/go/mcp-server-magictools/dist/mcp-server-magictools",
      "args": ["serve"],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin",
        "HOME": "/home/your-user"
      }
    }
  }
}
```

### 💻 VSCode (Roo Code / Cline)
**Paths:**
*   **macOS**: `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json`
*   **Linux**: `~/.config/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json`
*   **Windows**: `%APPDATA%\Code\User\globalStorage\rooveterinaryinc.roo-cline\settings\cline_mcp_settings.json`

```json
{
  "mcpServers": {
    "magictools": {
      "command": "/absolute/path/to/mcp-server-magictools",
      "args": ["serve"]
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
    "magictools": {
      "command": "/absolute/path/to/mcp-server-magictools",
      "args": ["serve"]
    }
  }
}
```

---

## 💻 CLI Commands Reference

| Command | Description | Usage |
| :--- | :--- | :--- |
| `serve` | Starts the MCP server (Stdio transport). | `./mcp-server-magictools serve` |
| `configure` | Launches the interactive configuration wizard. | `./mcp-server-magictools configure` |
| `db wipe` | Clears all tool and telemetry indices. | `./mcp-server-magictools db wipe` |
| `dashboard` | Starts the real-time TUI monitor. | `./mcp-server-magictools dashboard` |
| `-version` | Displays the current version. | `./mcp-server-magictools -version` |

---

## 💡 Use Cases

1.  **Autonomous Refactoring**: Use `execute_pipeline` to scan a Go project, identify complexity issues, and automatically apply idiomatic fixes.
2.  **Centralized Intelligence**: Instead of configuring 10 different MCP servers, configure MagicTools and let it manage everything.
3.  **Knowledge Retrieval**: Leverage the `recall` sub-server via MagicTools to retrieve relevant code patterns from your local codebase during a brainstorm session.
4.  **Watchdog Monitoring**: If a sub-server crashes, MagicTools automatically detects it and can reload it without interrupting your workflow.

---

*Built with ❤️ in Go for the Next Generation of AI Development.*

