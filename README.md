# 💾 Recall Memory Server

A high-performance Model Context Protocol (MCP) server providing AI agents with long-term, structured "memory"
for architectural decisions and project context.

## 🚀 Overview

The Recall Server bridges the gap between ephemeral AI context windows and persistent project requirements. It
provides a local-first, searchable key-value store that empowers agents to "remember" why decisions were made and
"recall" them precisely when needed.

### 📋 Core Pillars

1. **Knowledge Persistence**: Local-first storage for architectural patterns and institutional knowledge.
2. **Dual-Layer Organization**: Support for primary **Categories** and multi-value **Tags**.
3. **Intelligent Consolidation**: Automated redundancy detection and background context merging.
4. **Encryption-at-Rest**: Full cryptographic security for sensitive knowledge via AES-256.

---

## 🛠️ Tools

### `remember`

Stores a chunk of context with optional metadata.

- **Parameters**: `key`, `value`, `category`, `tags`

### `recall` / `search_memories`

Direct retrieval by key or semantic search across all saved memories.

### `ingest_files`

Structural ingestion of code files (.go) and documentation (.md) directly into the knowledge index using AST
parsing.

### `harvest_standards`

High-fidelity Go documentation harvesting and interface implementation mapping.

---

## ⚙️ Configuration

Control limits and security via environment variables:

| Variable | Description |
| :--- | :--- |
| `MCP_RECALL_DB_PATH` | Path to BadgerDB files. |
| `MCP_RECALL_ENCRYPTION_KEY` | 32-byte key for AES-256 encryption. |
| `MCP_RECALL_EXPORT_DIR` | Sandbox directory for import/export tools. |

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-recall main.go
```

### 2. Configure for IDEs

#### **Antigravity**

```yaml
mcpServers:
  recall:
    command: "/absolute/path/to/dist/mcp-server-recall"
    env:
      MCP_RECALL_ENCRYPTION_KEY: "your-32-char-key-here"
```

#### **VS Code / Cursor (mcp_config.json)**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/absolute/path/to/dist/mcp-server-recall",
      "args": [],
      "env": {
        "MCP_RECALL_DB_PATH": "/home/user/.config/mcp-server-recall",
        "MCP_RECALL_ENCRYPTION_KEY": "your-32-char-key-here"
      }
    }
  }
}
```

---

## 🔐 Security Note

If using encryption, generate a strictly 32-character key. If re-initializing a database, use the CLI:

```bash
./mcp-server-recall --reinit
```

---

*Built with Go for absolute privacy and transactional integrity.*
