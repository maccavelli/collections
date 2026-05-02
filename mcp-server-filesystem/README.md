# 📂 MagicFilesystem Sub-Server

A high-performance Model Context Protocol (MCP) sub-server providing secure, sandboxed filesystem operations.

## 🚀 Overview

`mcp-server-filesystem` provides a secure way for AI agents to interact with the local filesystem. It uses a "safe-list" approach, where only explicitly allowed directories are accessible to the agent.

### 📋 Core Pillars

1.  **Sandboxed Access**: Only directories passed as arguments (or via MCP roots) are accessible.
2.  **Path Normalization**: Automatically resolves symbolic links and prevents directory traversal attacks.
3.  **Comprehensive File Operations**: Supports reading, writing, moving, and listing files.
4.  **Metadata Inspection**: Provides tools for checking file stats and permissions.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`list_directory`**: Lists contents of an allowed directory.
*   **`read_file`**: Reads the content of a file within the sandbox.
*   **`write_to_file`**: Creates or overwrites a file with new content.
*   **`move_file`**: Safely renames or moves a file.
*   **`get_file_info`**: Retrieves metadata like size and modification time.

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

## ⚙️ Configuration

### 1. Build the Binary
```bash
make build
```

### 2. Execution Usage
You MUST specify the directories the server is allowed to access.
```bash
./dist/mcp-server-filesystem /path/to/project /path/to/docs
```

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "/absolute/path/to/mcp-server-filesystem",
      "args": ["/home/user/my-allowed-dir"]
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
    "filesystem": {
      "command": "C:/path/to/mcp-server-filesystem.exe",
      "args": ["C:/Users/Name/Documents"]
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
    "filesystem": {
      "command": "/usr/local/bin/mcp-server-filesystem",
      "args": ["/Users/Name/Projects"]
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
