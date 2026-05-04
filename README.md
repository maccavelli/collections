# 🪄 MagicTools Orchestrator

The central backbone of the Antigravity ecosystem. A robust, fault-tolerant Model Context Protocol (MCP) orchestrator,
gateway, and autonomous execution engine.

## 🚀 Overview

`mcp-server-magictools` is the **Master Architect** of the MCP swarm. It is designed to be the **primary** MCP
server configured in your IDE. Instead of managing dozens of individual servers, you point your IDE to MagicTools,
and it manages everything else.

### 📋 Core Pillars

1. **Unified Orchestration**: Manages the lifecycle of sub-servers (e.g., `go-refactor`, `brainstorm`, `recall`).
2. **Socratic DAG Engine**: Dynamically composes multi-step execution pipelines (Directed Acyclic Graphs) with
   built-in review gates.
3. **Hybrid Intelligence**: Employs **HNSW Vector Search** and **Bleve BM25 Lexical Search** for hyper-accurate
   tool alignment.
4. **Swarm Bidding**: When a tool is requested, the orchestrator polls all sub-servers to find the most capable
   handler for the specific intent.
5. **Observability**: Real-time TUI dashboard for monitoring latencies, process health, and telemetry streams.

---

## 🏗️ Architecture: How it Works

MagicTools sits between your LLM and your tool ecosystem:

1. **Intent Alignment**: When you ask "Refactor this Go function," the LLM calls `align_tools`. MagicTools uses its
   hybrid search engine to find the exact tools needed across all registered sub-servers.
2. **Pipeline Generation**: If the task is complex, `execute_pipeline` is invoked. MagicTools generates a DAG of
   tasks (e.g., scan -> analyze -> refactor -> verify -> report).
3. **Secure Proxying**: Every tool execution routes through `call_proxy`. This ensures that credentials, environment
   variables, and resource limits are enforced centrally.
4. **Circuit Breaking**: If a sub-server (like `recall`) becomes unresponsive, MagicTools isolates the failure and
   prevents it from crashing your entire IDE session.

---

## 🛠️ Getting Started: CLI Walkthrough

### 1. Build the Binary

Clone the repository and build the distribution:

```bash
make build
```

The binary will be available at `./dist/mcp-server-magictools`.

### 2. The Configuration Wizard

Run the interactive wizard to set up your LLM providers for search and embeddings:

```bash
./dist/mcp-server-magictools configure
```

**Walkthrough:**

* **Provider Selection**: Choose your primary LLM (Gemini is recommended for the best integration).
* **API Keys**: You will be prompted to enter your API keys. These are stored securely in `config.yaml`.
* **Embedding Engine**: Configure a vector provider (e.g., Voyage or Gemini) to enable semantic knowledge retrieval.
* **Storage**: The wizard creates your local workspace at `~/.config/mcp-server-magictools/` (Linux/macOS) or
  `%APPDATA%\mcp-server-magictools\` (Windows).

### 3. Verification

Verify your setup by launching the TUI dashboard:

```bash
./dist/mcp-server-magictools dashboard
```

You should see the status of the internal orchestrator handlers as "ONLINE".

---

## 🖥️ IDE Configuration Guide

MagicTools should be the **only** server in your configuration.

### 🌌 Antigravity / Gemini

**File Path:**

* **Linux/macOS**: `~/.gemini/mcp_config.json`
* **Windows**: `%APPDATA%\Gemini\antigravity\mcp_config.json`

**Configuration:**

```json
{
  "mcpServers": {
    "magictools": {
      "command": "/absolute/path/to/dist/mcp-server-magictools",
      "args": ["serve"],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/bin:/bin",
        "HOME": "/home/your-user"
      }
    }
  }
}
```

*Windows Note: Use double backslashes in paths, e.g., `"C:\\Users\\User\\...\\mcp-server-magictools.exe"`.*

### 💻 VSCode (Roo Code / Cline)

**File Path:**

* **Windows**: `%APPDATA%\Code\User\globalStorage\rooveterinaryinc.roo-cline\settings\cline_mcp_settings.json`
* **Linux**: `~/.config/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json`

**Configuration:**

```json
{
  "mcpServers": {
    "magictools": {
      "command": "C:\\path\\to\\mcp-server-magictools.exe",
      "args": ["serve"]
    }
  }
}
```

### 🤖 Claude Desktop

**File Path:**

* **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
* **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`

**Configuration:**

```json
{
  "mcpServers": {
    "magictools": {
      "command": "C:\\path\\to\\mcp-server-magictools.exe",
      "args": ["serve"]
    }
  }
}
```

---

## 💻 Advanced CLI Commands

| Command | Usage | Description |
| :--- | :--- | :--- |
| `serve` | `serve` | Starts the MCP server for IDE consumption. |
| `configure` | `configure` | Interactive setup for providers and keys. |
| `dashboard` | `dashboard` | Real-time TUI for health and telemetry. |
| `db wipe` | `db wipe` | Purges all local caches and indices. |
| `db sync` | `db sync` | Forces a re-index of all registered toolsets. |
| `servers list` | `servers list` | Lists all sub-servers managed by the orchestrator. |

---

## 💡 Best Practices

* **Single Point of Entry**: Never configure sub-servers like `go-refactor`
  directly in your IDE. Always route through MagicTools.
* **Warm the Cache**: After adding new sub-servers to `servers.yaml`, run
  `sync_ecosystem` via your IDE to update the search index.
* **Monitor Latency**: Use the `dashboard` during heavy refactoring tasks to
  ensure sub-servers are responding efficiently.
* **Context Management**: MagicTools automatically truncates long outputs to
  stay within LLM context limits while preserving important metadata headers.

---

*Built with ❤️ for the Next Generation of Agentic Coding.*
