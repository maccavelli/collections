# 📚 MagicRecall Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for long-term project memory,
semantic search, and codebase indexing.

## 🚀 Overview

`mcp-server-recall` is the "memory" of your AI agent. It crawls your project, standard
libraries, and documentation to create a high-performance vector and keyword index, enabling rapid
retrieval of relevant context during development.

### 📋 Core Pillars

* **Unified Search**: Combines keyword search (Bleve) and semantic vector search for maximum accuracy.
* **Context Harvesting**: Automatically crawls directories and Go packages to build a knowledge graph.
* **Encrypted at Rest**: Optionally protects your indexed data using AES-256-GCM encryption.
* **TUI Dashboard**: Provides a real-time monitor to inspect index health and search latency.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`search_sessions`**: Searches across past conversations and project context.
* **`get_context_chunk`**: Retrieves a specific snippet of code or documentation from the index.
* **`harvest_directory`**: Manually triggers a scan of a specific path to update the index.
* **`purge_data`**: Removes stale or irrelevant data from the memory bank.

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

## 📦 Installation & Setup

### 1. Build & Initial Configuration

```bash
make build
./dist/mcp-server-recall configure
```

**MANDATORY:** You must run the `configure` command first to set up encryption and generate the
initial `recall.yaml` configuration.

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run Recall as an orchestrated
sub-server:

```yaml
- name: recall
  command: /absolute/path/to/mcp-server-recall
  args:
    - serve
  env:
    HOME: /absolute/path/to/home
  memory_limit_mb: 4096
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
    "recall": {
      "command": "/absolute/path/to/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/absolute/path/to/home"
      }
    }
  }
}
```

##### Windows Example (Antigravity Suite)

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\path\\to\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "HOME": "C:\\Users\\YourName"
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
    "recall": {
      "command": "/absolute/path/to/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/absolute/path/to/home"
      }
    }
  }
}
```

##### Windows Example (Claude Suite)

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\path\\to\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "HOME": "C:\\Users\\YourName"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
