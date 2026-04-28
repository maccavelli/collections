# 🧠 Sequential Thinking MCP Server

A specialized Model Context Protocol (MCP) server for dynamic and reflective problem-solving through structured
cognitive steps.

## 🚀 Overview

The Sequential Thinking server helps AI agents systematically reason through complex problems. By breaking
problems down into sequentially linked thoughts, it provides out-of-band context tracking that supports
branching, revising, and course correction.

### 📋 Core Pillars

1. **Step-by-Step Breakdown**: Enforces a logical progression for planning and design.
2. **Dynamic Course Correction**: Allows agents to revise previous thoughts as new insights emerge.
3. **Hypothesis Branching**: Tracks alternate lines of reasoning without losing the main thread.
4. **Context Filtering**: Maintains long-term reasoning state while filtering out irrelevant noise.

---

## 🛠️ Tools

### `sequentialthinking`

The primary reasoning tool for dynamic context tracking.

- **Parameters**:
  - `thought`: Current reasoning content.
  - `thoughtNumber` / `totalThoughts`: Progress estimation.
  - `isRevision` / `revisesThought`: Revision metadata.
  - `branchId` / `branchFromThought`: Branching metadata.

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-sequential-thinking main.go
```

### 2. Configure for IDEs

#### **Antigravity**

```yaml
mcpServers:
  seq-thinking:
    command: "/absolute/path/to/dist/mcp-server-sequential-thinking"
```

#### **VS Code (MCP Extension / Cline)**

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "seq-thinking": {
      "command": "/absolute/path/to/dist/mcp-server-sequential-thinking",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

#### **Cursor IDE**

1. **Settings** -> **Features** -> **MCP**.
2. **+ Add New MCP Server**.
3. Name: `Sequential Thinking`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-sequential-thinking`

---

## 📖 Use Cases

- **Complex Troubleshooting**: Test multiple bug hypotheses simultaneously using branches.
- **Architectural Planning**: Break down refactors into logical steps with clear revision paths.
- **Signal Extraction**: Filter signal from noise when dealing with massive context payloads.

---

## 💻 CLI Functionality

Check version and engine status:

```bash
./mcp-server-sequential-thinking -version
```

---

*Built with Go for performance and strict cognitive determinism.*
