# 🧠 MagicBrainstorm Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for deep architectural brainstorming, requirement exploration, and project discovery.

## 🚀 Overview

`mcp-server-brainstorm` is a core sub-server of the MagicTools suite. It helps engineers stress-test designs, identify project gaps, and document decisions through Socratic analysis and adversarial personas.

### 📋 Core Pillars

1.  **Requirement Synthesis**: Breaks down high-level requests into technical specifications.
2.  **Context-Aware Analysis**: Scans project structure and tech stacks to identify documentation gaps.
3.  **Adversarial Persona (Red Teaming)**: Audits designs for scalability, security, and modularity.
4.  **Decision Records**: Translates discussions into structured Architecture Decision Records (ADRs).

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`clarify_requirements`**: Detects architectural "decision forks" in your requirements.
*   **`discover_project`**: Performs a unified scan of your tech stack and structure.
*   **`critique_design`**: Multi-dimensional assessment of a design snippet.
*   **`capture_decision_logic`**: Generates ADRs from your brainstorming sessions.

### Orchestration with MagicTools (Recommended)
MagicTools manages Brainstorm's lifecycle. Invoke its tools via `magictools:call_proxy`:
```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "brainstorm:clarify_requirements",
    "arguments": { "requirements": "build a distributed cache" }
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
If running without the MagicTools orchestrator:
| Variable | Description |
| :--- | :--- |
| `MCP_API_URL` | Comma-separated list of context servers (e.g., `recall`). |
| `MCP_ORCHESTRATOR_OWNED` | Set to `true` for full swarm integration. |

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "brainstorm": {
      "command": "/absolute/path/to/mcp-server-brainstorm",
      "env": {
        "MCP_ORCHESTRATOR_OWNED": "true"
      }
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
    "brainstorm": {
      "command": "C:/path/to/mcp-server-brainstorm.exe"
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
    "brainstorm": {
      "command": "/usr/local/bin/mcp-server-brainstorm"
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
