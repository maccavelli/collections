# ✨ MagicSkills Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for managing, discovering, and delivering executable AI skills.

## 🚀 Overview

`mcp-server-magicskills` is the knowledge repository for the MagicTools ecosystem. It manages "Skills"—structured markdown files containing technical directives, best practices, and execution patterns that govern agent behavior.

### 📋 Core Pillars

1.  **Dynamic Skill Matching**: Uses semantic alignment to find the most relevant skill for a given task.
2.  **Directive Extraction**: Retrieves specific sections of a skill (e.g., "Execution Constraints") to minimize context window usage.
3.  **Cross-Session Persistence**: Maintains a standard set of skills (like the `default` skill) across all AI interactions.
4.  **Optimized Delivery**: Uses a specialized transport protocol to deliver large skill documents without hitting IDE size limits.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`magicskills_match`**: Searches the skill repository for the best match for an intent.
*   **`magicskills_get`**: Retrieves the full or partial content of a specific skill.
*   **`magicskills_list`**: Provides an index of all available skills in the current environment.

### Orchestration with MagicTools (Recommended)
Invoke MagicSkills tools via `magictools:call_proxy`:
```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "magicskills:magicskills_get",
    "arguments": { "name": "default", "section": "orchestrator constraints" }
  }
}
```

---

## ⚙️ Configuration

### 1. Build the Binary
```bash
make build
```

### 2. CLI Options
| Option | Description |
| :--- | :--- |
| `-db` | Path to the BadgerDB data directory (default: platform-native). |
| `-debug` | Enable full trace logging for diagnostic purposes. |
| `-no-optimize` | Disables minification and SqueezeWriter optimizations. |
| `-version` | Print version info and exit. |

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/absolute/path/to/mcp-server-magicskills",
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
    "magicskills": {
      "command": "C:/path/to/mcp-server-magicskills.exe",
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
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
      "args": ["serve"]
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
