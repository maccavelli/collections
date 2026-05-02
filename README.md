# 🧠 MagicSequential-Thinking Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for structured, deep cognitive reasoning and self-correcting logic loops.

## 🚀 Overview

`mcp-server-sequential-thinking` provides a profound cognitive processing lattice. It is designed to help AI agents map intricate paradoxes, unravel complex logic topologies, and perform rigorous self-critique before committing to terminal actions.

### 📋 Core Pillars

1.  **Iterative Reasoning**: Forces the agent to think through multiple steps, documenting assumptions and revisions.
2.  **Self-Critique**: Mandatory self-correction phase in every thought step to identify flaws or assumptions.
3.  **Branching Logic**: Allows the agent to explore multiple reasoning paths (branches) and merge them back into a final insight.
4.  **Resource Persistence**: Tracks the entire thinking process to ensure no cognitive context is lost during long tasks.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`sequentialthinking`**: The primary reasoning tool. It requires a detailed thought, self-critique, and a determination if more thoughts are needed.

### Orchestration with MagicTools (Recommended)
Invoke Sequential-Thinking tools via `magictools:call_proxy`:
```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "seq-thinking:sequentialthinking",
    "arguments": {
      "thought": "I should refactor the auth system to use JWT",
      "selfCritique": "JWT might be overkill for this local tool",
      "contradictionDetected": false,
      "nextThoughtNeeded": true,
      "thoughtNumber": 1,
      "totalThoughts": 5
    }
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
| `-version` | Print version info and exit. |

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "sequential-thinking": {
      "command": "/absolute/path/to/mcp-server-sequential-thinking",
      "args": []
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
    "sequential-thinking": {
      "command": "C:/path/to/mcp-server-sequential-thinking.exe"
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
    "sequential-thinking": {
      "command": "/usr/local/bin/mcp-server-sequential-thinking"
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
