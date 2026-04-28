# 🪄 MagicTools Orchestrator

The central backbone of the Antigravity ecosystem. A robust, fault-tolerant Model Context Protocol (MCP)
orchestrator and gateway.

## 🚀 Overview

The `mcp-server-magictools` orchestrator is the primary gateway that binds native tools and external servers into
a single context. It ensures deterministic LLM interactions, provides deep system telemetry, and enforces safety
protocols in real-time.

### 📋 Core Pillars

1. **Sub-server Management**: Hot-loads and manages external MCP sub-servers dynamically (e.g., `go-refactor`,
   `brainstorm`).
2. **Socratic DAG Generation**: Dynamically composes execution pipelines for complex tasks using topological
   sorting.
3. **Telemetry & Indexing**: Employs BadgerDB and Bleve for high-performance tool indexing and usage tracking.
4. **Safety & Resource Limits**: Enforces rigid memory bounds (1024MiB) and strict concurrency pipelines via
   `errgroup`.

---

## 🛠️ Specialized Tools

### `align_tools`

The primary intent mapping engine. Finds the best tools for a specific task URN.

### `compose_pipeline`

Generates a DAG execution plan for brainstorm and refactor analysis.

### `call_proxy`

The absolute execution endpoint for orchestrating downstream MCP nodes natively.

### `sync_ecosystem`

Synchronizes all managed sub-servers to globally refresh tool indices.

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-magictools main.go
```

### 2. Configure for IDEs

#### **Antigravity**

MagicTools is the native orchestrator for Antigravity. It is typically launched as part of the core environment.

#### **VS Code (MCP Extension / Cline)**

```json
{
  "mcpServers": {
    "magictools": {
      "command": "/absolute/path/to/dist/mcp-server-magictools",
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
3. Name: `MagicTools`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-magictools`

---

## 📊 Telemetry & Monitoring

MagicTools provides real-time health reports and performance metrics for all connected sub-servers. Use
`get_health_report` or `analyze_system_logs` to monitor the ecosystem.

---

## 💻 CLI Functionality

Check version or wipe search indices:

```bash
./mcp-server-magictools -version
./mcp-server-magictools db wipe
```

---

*The master architect of the MCP Swarm. Built with Go.*
