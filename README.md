# ✨ MagicSkills Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for managing, discovering, and
delivering executable AI skills.

## 🚀 Overview

`mcp-server-magicskills` is the knowledge repository for the MagicTools ecosystem. It manages
"Skills"—structured markdown files containing technical directives, best practices, and execution
patterns that govern agent behavior.

### 📋 Core Pillars

* **Dynamic Skill Matching**: Uses semantic alignment to find the most relevant skill for a given task.
* **Directive Extraction**: Retrieves specific sections of a skill (e.g., "Execution
  Constraints") to minimize context window usage.
* **Cross-Session Persistence**: Maintains a standard set of skills (like the `default` skill) across all AI interactions.
* **Optimized Delivery**: Uses a specialized transport protocol to deliver large skill
  documents without hitting IDE size limits.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`magicskills_match`**: Searches the skill repository for the best match for an intent.
* **`magicskills_get`**: Retrieves the full or partial content of a specific skill.
* **`magicskills_list`**: Provides an index of all available skills in the current environment.

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

## 📦 Installation & Setup

### 1. Build the Binary

```bash
make build
```

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run MagicSkills as an orchestrated
sub-server:

```yaml
- name: magicskills
  command: /absolute/path/to/mcp-server-magicskills
  args:
    - serve
  env:
    HOME: /absolute/path/to/home
    MCP_API_URL: http://localhost:7000/mcp # URL for Recall context
  memory_limit_mb: 2048
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
    "magicskills": {
      "command": "/absolute/path/to/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp"
      }
    }
  }
}
```

##### Windows Example (Antigravity Suite)

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\path\\to\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "HOME": "C:\\Users\\YourName",
        "MCP_API_URL": "http://localhost:7000/mcp"
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
    "magicskills": {
      "command": "/absolute/path/to/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp"
      }
    }
  }
}
```

##### Windows Example (Claude Suite)

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\path\\to\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "HOME": "C:\\Users\\YourName",
        "MCP_API_URL": "http://localhost:7000/mcp"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
