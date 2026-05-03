# 🧠 MagicBrainstorm Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for deep architectural brainstorming,
requirement exploration, and project discovery.

## 🚀 Overview

`mcp-server-brainstorm` is a core sub-server of the MagicTools suite. It helps engineers stress-test designs,
identify project gaps, and document decisions through Socratic analysis and adversarial personas.

### 📋 Core Pillars

* **Requirement Synthesis**: Breaks down high-level requests into technical specifications.
* **Context-Aware Analysis**: Scans project structure and tech stacks to identify documentation gaps.
* **Adversarial Persona (Red Teaming)**: Audits designs for scalability, security, and modularity.
* **Decision Records**: Translates discussions into structured Architecture Decision Records (ADRs).

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`clarify_requirements`**: Detects architectural "decision forks" in your requirements.
* **`discover_project`**: Performs a unified scan of your tech stack and structure.
* **`critique_design`**: Multi-dimensional assessment of a design snippet.
* **`capture_decision_logic`**: Generates ADRs from your brainstorming sessions.

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

## 📦 Installation & Setup

### 1. Build the Binary

```bash
make build
```

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run Brainstorm as an orchestrated sub-server:

```yaml
- name: brainstorm
  command: /absolute/path/to/mcp-server-brainstorm
  env:
    HOME: /absolute/path/to/home
    MCP_API_URL: http://localhost:7000/mcp # URL for Recall context
  memory_limit_mb: 4096
  gomemlimit_mb: 2048
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
    "brainstorm": {
      "command": "/absolute/path/to/mcp-server-brainstorm",
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp"
      }
    }
  }
}
```

##### Windows Example (Antigravity)

```json
{
  "mcpServers": {
    "brainstorm": {
      "command": "C:\\path\\to\\mcp-server-brainstorm.exe",
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
    "brainstorm": {
      "command": "/absolute/path/to/mcp-server-brainstorm",
      "env": {
        "HOME": "/absolute/path/to/home",
        "MCP_API_URL": "http://localhost:7000/mcp"
      }
    }
  }
}
```

##### Windows Example (Claude)

```json
{
  "mcpServers": {
    "brainstorm": {
      "command": "C:\\path\\to\\mcp-server-brainstorm.exe",
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
