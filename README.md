# 📂 MagicFilesystem Sub-Server

A high-performance Model Context Protocol (MCP) sub-server providing secure, sandboxed
filesystem operations.

## 🚀 Overview

`mcp-server-filesystem` provides a secure way for AI agents to interact with the local filesystem.
It uses a "safe-list" approach, where only explicitly allowed directories are accessible to the agent.

### 📋 Core Pillars

* **Sandboxed Access**: Only directories passed as arguments (or via MCP roots) are accessible.
* **Path Normalization**: Automatically resolves symbolic links and prevents directory traversal attacks.
* **Comprehensive File Operations**: Supports reading, writing, moving, and listing files.
* **Metadata Inspection**: Provides tools for checking file stats and permissions.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`list_directory`**: Lists contents of an allowed directory.
* **`read_file`**: Reads the content of a file within the sandbox.
* **`write_to_file`**: Creates or overwrites a file with new content.
* **`move_file`**: Safely renames or moves a file.
* **`get_file_info`**: Retrieves metadata like size and modification time.

### Orchestration with MagicTools (Recommended)

Invoke Filesystem tools via `magictools:call_proxy`:

```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "filesystem:list_directory",
    "arguments": { "path": "/home/user/project" }
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

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run Filesystem as an orchestrated
sub-server. **Note:** You must provide at least one allowed directory as an argument.

```yaml
- name: filesystem
  command: /absolute/path/to/mcp-server-filesystem
  args:
    - /home/user/gitrepos
    - /home/user/.local
  env:
    HOME: /absolute/path/to/home
  memory_limit_mb: 1024
  max_cpu_limit: 2
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
    "filesystem": {
      "command": "/absolute/path/to/mcp-server-filesystem",
      "args": [
        "/Users/YourName/Projects",
        "/Users/YourName/Downloads"
      ],
      "env": {
        "HOME": "/absolute/path/to/home"
      }
    }
  }
}
```

##### Windows Example (Antigravity)

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "C:\\path\\to\\mcp-server-filesystem.exe",
      "args": [
        "C:\\Users\\YourName\\Documents",
        "C:\\Users\\YourName\\AppData\\Local\\Temp"
      ],
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
    "filesystem": {
      "command": "/absolute/path/to/mcp-server-filesystem",
      "args": [
        "/Users/YourName/Projects",
        "/Users/YourName/Downloads"
      ],
      "env": {
        "HOME": "/absolute/path/to/home"
      }
    }
  }
}
```

##### Windows Example (Claude)

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "C:\\path\\to\\mcp-server-filesystem.exe",
      "args": [
        "C:\\Users\\YourName\\Documents",
        "C:\\Users\\YourName\\AppData\\Local\\Temp"
      ],
      "env": {
        "HOME": "C:\\Users\\YourName"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
