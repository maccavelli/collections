# 🧠 MagicSequential-Thinking Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for structured, deep cognitive
reasoning and self-correcting logic loops.

## 🚀 Overview

`mcp-server-sequential-thinking` provides a profound cognitive processing lattice. It is
designed to help AI agents map intricate paradoxes, unravel complex logic topologies, and perform
rigorous self-critique before committing to terminal actions.

### 📋 Core Pillars

* **Iterative Reasoning**: Forces the agent to think through multiple steps, documenting assumptions and revisions.
* **Self-Critique**: Mandatory self-correction phase in every thought step to identify flaws or assumptions.
* **Branching Logic**: Allows the agent to explore multiple reasoning paths (branches)
  and merge them back into a final insight.
* **Resource Persistence**: Tracks the entire thinking process to ensure no cognitive context is lost during long tasks.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`sequentialthinking`**: The primary reasoning tool. It requires a detailed thought,
  self-critique, and a determination if more thoughts are needed.

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

## 📦 Installation & Setup

### 1. Build the Binary

```bash
make build
```

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run Sequential-Thinking as an
orchestrated sub-server:

```yaml
- name: seq-thinking
  command: /absolute/path/to/mcp-server-sequential-thinking
  env:
    HOME: /absolute/path/to/home
  memory_limit_mb: 512
  max_cpu_limit: 1
  disabled: false
  deferred_boot: true
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
    "sequential-thinking": {
      "command": "/absolute/path/to/mcp-server-sequential-thinking",
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
    "sequential-thinking": {
      "command": "C:\\path\\to\\mcp-server-sequential-thinking.exe",
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
    "sequential-thinking": {
      "command": "/absolute/path/to/mcp-server-sequential-thinking",
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
    "sequential-thinking": {
      "command": "C:\\path\\to\\mcp-server-sequential-thinking.exe",
      "env": {
        "HOME": "C:\\Users\\YourName"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
