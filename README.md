# ✨ MagicSkills MCP Server

A specialized Model Context Protocol (MCP) server for high-density knowledge retrieval, skill discovery, and
automated workflow bootstrapping.

## 🚀 Overview

The MagicSkills server manages a sophisticated repository of expert-level "skills"—complex, multi-step procedures
and domain knowledge. It acts as a long-term memory and procedural accelerator for developers and AI agents.

### 📋 Core Pillars

1. **Skill Discovery & Matching**: Intelligent searching for relevant procedures using semantic matching (BM25)
   and tag filtering.
2. **Density & Compression**: Refines large, verbose documentation into "Dense Summaries" optimized for LLM
   context windows.
3. **Workflow Bootstrapping**: Automatically extracts checklists and validation steps from skill definitions.
4. **Host Verification**: Audits and validates the necessary environment dependencies required for a specific skill.

---

## 🛠️ Tools

### `magicskills_list`

Provides a comprehensive list of all skills available in the current index.

### `magicskills_match`

Automatically finds the best-matching skills for a given goal and returns a dense digest.

- **Parameter**: `intent` (string)

### `magicskills_get`

Fetches high-relevance expert knowledge for a specific skill.

- **Parameters**: `name` (string), `section` (string, optional), `version` (string, optional)

### `magicskills_validate_deps`

Checks the host environment for required binary dependencies for a skill.

- **Parameter**: `name` (string)

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-magicskills main.go
```

### 2. Configure for IDEs

#### **Antigravity**

```yaml
mcpServers:
  magicskills:
    command: "/absolute/path/to/dist/mcp-server-magicskills"
```

#### **VS Code (MCP Extension / Cline)**

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/absolute/path/to/dist/mcp-server-magicskills",
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
3. Name: `MagicSkills`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-magicskills`

---

## 📖 Use Cases

- **Intent-Based Assistance**: Automatically find the right troubleshooting guide for a specific error.
- **Knowledge Transfer**: Capture senior developer knowledge into machine-readable skills for universal reuse.
- **Dependency Guarding**: Ensure all team members have required tools installed before starting a task.

---

## 💻 CLI Functionality

Check version and manage roots:

```bash
./mcp-server-magicskills -version
```

---

*Built with Go for maximum knowledge density and speed.*
