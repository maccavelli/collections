# MagicRecall MCP Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for long-term project memory, semantic search, and codebase indexing.

## Overview

`mcp-server-recall` is the "memory" of your AI agent. It crawls your project, standard libraries, and documentation to create a high-performance vector and keyword index, enabling rapid retrieval of relevant context during development.

### Core Capabilities

| Feature | Description |
|---|---|
| **Unified Search** | Combines keyword search (Bleve) and semantic vector search for maximum accuracy. |
| **Context Harvesting** | Automatically crawls directories and Go packages to build a knowledge graph. |
| **Encrypted at Rest** | Optionally protects your indexed data using AES-256-GCM encryption. |
| **TUI Dashboard** | Provides a real-time monitor to inspect index health and search latency. |

---

## Quick Start

### Step 1: Place the Binary

Download the `mcp-server-recall` binary for your platform and place it in a directory on your system `PATH`.

#### Linux

```bash
# Move the binary to your local bin directory
mv mcp-server-recall ~/.local/bin/mcp-server-recall
chmod +x ~/.local/bin/mcp-server-recall
```

#### macOS

```bash
# Move the binary to your local bin directory
mv mcp-server-recall /usr/local/bin/mcp-server-recall
chmod +x /usr/local/bin/mcp-server-recall
```

#### Windows (PowerShell)

```powershell
# Create a directory for the binary if it doesn't exist
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\Programs\recall"

# Move the binary
Move-Item mcp-server-recall.exe "$env:LOCALAPPDATA\Programs\recall\mcp-server-recall.exe"

# Add to your PATH (current user, persistent)
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$currentPath;$env:LOCALAPPDATA\Programs\recall", "User")
```

---

### Step 2: Initialize Configuration

Generate the default `recall.yaml` configuration file. This is required before the server can start.

```bash
mcp-server-recall init
```

### Step 3: Configure Integrations

Run the interactive configuration wizard to set up encryption keys, indexing paths, and vector storage preferences.

```bash
mcp-server-recall configure
```

---

### Step 4: Configure Your IDE

> **⚠️ IMPORTANT ORCHESTRATOR MESSAGING**
> 
> While the standalone IDE configurations below are provided for testing and debugging, `mcp-server-recall` is designed to be run as a downstream node behind the **`magictools` orchestrator** in production environments. 
> 
> When running in production, you should **only** configure `magictools` in your IDE, which will automatically proxy requests to `recall` as needed.

If you are testing the server standalone, configure your IDE to launch the binary directly. Note that you **must** pass the `serve` argument.

#### Antigravity (Google DeepMind)

| OS | Configuration File Path |
|---|---|
| Linux | `~/.gemini/antigravity/mcp_config.json` |
| macOS | `~/.gemini/antigravity/mcp_config.json` |
| Windows | `%USERPROFILE%\.gemini\antigravity\mcp_config.json` |

**Linux / macOS:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/home/youruser/.local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\recall\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

#### Visual Studio Code (GitHub Copilot / Native MCP)

| OS | User-Level Configuration File Path |
|---|---|
| Linux | `~/.config/Code/User/mcp.json` |
| macOS | `~/Library/Application Support/Code/User/mcp.json` |
| Windows | `%APPDATA%\Code\User\mcp.json` |

**Linux:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/home/youruser/.local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\recall\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

#### VSCode — Cline Extension

| OS | Configuration File Path |
|---|---|
| Linux | `~/.cline/data/settings/cline_mcp_settings.json` |
| macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json` |

**Linux:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/home/youruser/.local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\recall\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

#### Claude Desktop

| OS | Configuration File Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

**macOS:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\recall\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

#### Claude Code (CLI)

Claude Code uses a CLI command to register MCP servers.

**Linux:**
```bash
claude mcp add recall -s user -- /home/youruser/.local/bin/mcp-server-recall serve
```

**macOS:**
```bash
claude mcp add recall -s user -- /usr/local/bin/mcp-server-recall serve
```

**Windows (PowerShell):**
```powershell
claude mcp add recall -s user -- "C:\Users\YourUser\AppData\Local\Programs\recall\mcp-server-recall.exe" serve
```

#### Cursor

| OS | Global Configuration File Path |
|---|---|
| Linux | `~/.cursor/mcp.json` |
| macOS | `~/.cursor/mcp.json` |
| Windows | `%USERPROFILE%\.cursor\mcp.json` |

**Linux:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/home/youruser/.local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\recall\\mcp-server-recall.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

---

## MCP Tools Reference

Once the server is running, the following tools are exposed to your IDE's AI agent:

| Tool | Description |
|---|---|
| `search_sessions` | Searches across past conversations and project context. |
| `get_context_chunk` | Retrieves a specific snippet of code or documentation from the index. |
| `harvest_directory` | Manually triggers a scan of a specific path to update the index. |
| `purge_data` | Removes stale or irrelevant data from the memory bank. |

---

## CLI Commands Reference

### `init`

Generates the default `recall.yaml` configuration file. Safe to run multiple times.

```bash
mcp-server-recall init
```

### `configure`

Opens an interactive menu to set up encryption keys, indexing paths, and vector storage preferences.

```bash
mcp-server-recall configure
```

### `serve`

Starts the MCP server over stdio. This is the command your IDE calls to launch the server.

```bash
mcp-server-recall serve
```

### `dashboard`

Launches the TUI (Terminal User Interface) dashboard to monitor index health and search latency in real-time.

```bash
mcp-server-recall dashboard
```

### `harvest`

Manually trigger the context harvesting engine from the command line.

```bash
mcp-server-recall harvest
```

### `purge`

Purges stale entries from the Bleve/Badger indices.

```bash
mcp-server-recall purge
```

---

## Data Storage Locations

| Data | Linux | macOS | Windows |
|---|---|---|---|
| Configuration | `~/.config/mcp-server-recall/recall.yaml` | `~/Library/Application Support/mcp-server-recall/recall.yaml` | `%APPDATA%\mcp-server-recall\recall.yaml` |
| Database (Bleve/Badger) | `~/.cache/mcp-server-recall/` | `~/Library/Caches/mcp-server-recall/` | `%LOCALAPPDATA%\mcp-server-recall\` |
| Server Logs | `stderr` (captured by IDE) | `stderr` (captured by IDE) | `stderr` (captured by IDE) |

---

*Built with Go. Part of the MagicTools Intelligence Suite.*
