# 📚 MagicRecall Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for long-term project memory, semantic search, and codebase indexing.

## 🚀 Overview

`mcp-server-recall` is the "memory" of your AI agent. It crawls your project, standard libraries, and documentation to create a high-performance vector and keyword index, enabling rapid retrieval of relevant context during development.

### 📋 Core Pillars

1.  **Unified Search**: Combines keyword search (Bleve) and semantic vector search for maximum accuracy.
2.  **Context Harvesting**: Automatically crawls directories and Go packages to build a knowledge graph.
3.  **Encrypted at Rest**: Optionally protects your indexed data using AES-256-GCM encryption.
4.  **TUI Dashboard**: Provides a real-time monitor to inspect index health and search latency.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`search_sessions`**: Searches across past conversations and project context.
*   **`get_context_chunk`**: Retrieves a specific snippet of code or documentation from the index.
*   **`harvest_directory`**: Manually triggers a scan of a specific path to update the index.
*   **`purge_data`**: Removes stale or irrelevant data from the memory bank.

### Orchestration with MagicTools (Recommended)
Invoke Recall tools via `magictools:call_proxy`:
```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "recall:search_sessions",
    "arguments": { "query": "how is the auth system implemented?" }
  }
}
```

---

## ⚙️ Initial Configuration

You MUST use the `configure` CLI command to set up encryption and the initial configuration file.

### 1. Build the Binary
```bash
make build
```

### 2. Run the Configuration Wizard
```bash
./dist/mcp-server-recall configure
```
**What this does:**
*   **Encryption Setup**: Generates or imports an AES-256 key for database protection.
*   **Template Generation**: Creates a standard `recall.yaml` configuration in your platform-native config directory (e.g., `~/.config/mcp-server-recall/`).

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "recall": {
      "command": "/absolute/path/to/mcp-server-recall",
      "args": ["serve"]
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
    "recall": {
      "command": "C:/path/to/mcp-server-recall.exe",
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
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": ["serve"]
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
