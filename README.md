# MagicSkills MCP Server

A high-performance Model Context Protocol (MCP) server for managing, discovering, and delivering executable AI skills.

## Overview

`mcp-server-magicskills` is the knowledge repository for the MagicTools ecosystem. It manages "Skills"—structured markdown files containing technical directives, best practices, and execution patterns that govern agent behavior.

### Core Capabilities

| Feature | Description |
|---|---|
| **Dynamic Skill Matching** | Uses semantic alignment to find the most relevant skill for a given task. |
| **Directive Extraction** | Retrieves specific sections of a skill (e.g., "Execution Constraints") to minimize context window usage. |
| **Cross-Session Persistence** | Maintains a standard set of skills (like the `default` skill) across all AI interactions. |
| **Optimized Delivery** | Uses a specialized transport protocol to deliver large skill documents without hitting IDE size limits. |

---

## Quick Start

### Step 1: Place the Binary

Download the `mcp-server-magicskills` binary for your platform and place it in a directory on your system `PATH`.

#### Linux

```bash
# Move the binary to your local bin directory
mv mcp-server-magicskills ~/.local/bin/mcp-server-magicskills
chmod +x ~/.local/bin/mcp-server-magicskills
```

#### macOS

```bash
# Move the binary to your local bin directory
mv mcp-server-magicskills /usr/local/bin/mcp-server-magicskills
chmod +x /usr/local/bin/mcp-server-magicskills
```

#### Windows (PowerShell)

```powershell
# Create a directory for the binary if it doesn't exist
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\Programs\magicskills"

# Move the binary
Move-Item mcp-server-magicskills.exe "$env:LOCALAPPDATA\Programs\magicskills\mcp-server-magicskills.exe"

# Add to your PATH (current user, persistent)
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$currentPath;$env:LOCALAPPDATA\Programs\magicskills", "User")
```

---

### Step 2: Initialize Configuration

`mcp-server-magicskills` does not require local configuration files, API tokens, or initialization steps like `init` or `configure`. It will automatically scan for skills in your workspace.

---

### Step 3: Configure Your IDE

Configure your IDE to launch the binary directly. Note that you **must** pass the `serve` argument, and you should define `MCP_API_URL` if you want to enable the integration with the context server.

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
    "magicskills": {
      "command": "/home/youruser/.local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicskills\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser",
        "MCP_API_URL": "http://localhost:18001/mcp"
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
    "magicskills": {
      "command": "/home/youruser/.local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicskills\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser",
        "MCP_API_URL": "http://localhost:18001/mcp"
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
    "magicskills": {
      "command": "/home/youruser/.local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicskills\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser",
        "MCP_API_URL": "http://localhost:18001/mcp"
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
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicskills\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

#### Claude Code (CLI)

Claude Code uses a CLI command to register MCP servers.

**Linux:**
```bash
claude mcp add magicskills -s user -- /home/youruser/.local/bin/mcp-server-magicskills serve
```

**macOS:**
```bash
claude mcp add magicskills -s user -- /usr/local/bin/mcp-server-magicskills serve
```

**Windows (PowerShell):**
```powershell
claude mcp add magicskills -s user -- "C:\Users\YourUser\AppData\Local\Programs\magicskills\mcp-server-magicskills.exe" serve
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
    "magicskills": {
      "command": "/home/youruser/.local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser",
        "MCP_API_URL": "http://localhost:18001/mcp"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicskills\\mcp-server-magicskills.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser",
        "MCP_API_URL": "http://localhost:18001/mcp"
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
| `magicskills_match` | Searches the skill repository for the best match for an intent. |
| `magicskills_get` | Retrieves the full or partial content of a specific skill. |
| `magicskills_list` | Provides an index of all available skills in the current environment. |
| `magicskills_add_root` | Adds a new directory to the skill search path. |
| `magicskills_sync_skills` | Forces a refresh and synchronization of the master file directory cache. |

---

## CLI Commands Reference

### `serve`

Starts the MCP server over stdio. This is the command your IDE calls to launch the server.

```bash
mcp-server-magicskills serve
```

---

## Data Storage Locations

| Data | Linux | macOS | Windows |
|---|---|---|---|
| Database Cache (Bleve) | `~/.cache/mcp-server-magicskills/` | `~/Library/Caches/mcp-server-magicskills/` | `%LOCALAPPDATA%\mcp-server-magicskills\` |
| Server Logs | `stderr` (captured by IDE) | `stderr` (captured by IDE) | `stderr` (captured by IDE) |

---

*Built with Go. Part of the MagicTools Intelligence Suite.*
