# 📂 Filesystem MCP Server

A specialized Model Context Protocol (MCP) server providing safe, sandboxed filesystem operations with strict path
boundaries and atomic write guarantees.

## 🚀 Overview

The Filesystem server provides AI agents with a robust interface for local file manipulation. It is designed with
safety as a first-class citizen, ensuring that operations are confined to allowed directories while providing advanced
features like atomic writes and media support.

### 📋 Core Pillars

1. **Sandboxed Execution**: All operations are strictly constrained to a whitelist of allowed directories.
2. **Atomic Integrity**: Uses temporary files and rename mechanisms to ensure robust writes and prevent corruption.
3. **Cross-Platform Fidelity**: Consistent path handling and normalization across Windows, Linux, and macOS.
4. **Rich Media Support**: Capable of reading text, multi-file batches, and rendering media (images/audio) as base64
   payloads.

---

## 🛠️ Tools

The server exposes 14 tools including:

- **`read_text_file`**: Read a file with optional line-range bounding.
- **`write_file`**: Atomic single-file write operations.
- **`edit_file`**: Line-based structural diff editing (supports flexible matching).
- **`list_directory`**: Granular file listing with metadata (sizes, types).
- **`directory_tree`**: Full recursive JSON tree builder with exclusion support.
- **`search_files`**: Glob-based pattern matching within the sandbox.
- **`get_file_info`**: Detailed OS stat metadata (permissions, dates, sizes).

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-filesystem main.go
```

### 2. Configure for IDEs

#### **Antigravity**

Configure allowed directories via positional arguments:

```yaml
mcpServers:
  filesystem:
    command: "/absolute/path/to/dist/mcp-server-filesystem"
    args: ["/path/to/allowed/dir1", "/path/to/allowed/dir2"]
```

#### **VS Code (MCP Extension / Cline)**

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "/absolute/path/to/dist/mcp-server-filesystem",
      "args": ["/home/user/projects"],
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
3. Name: `Filesystem`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-filesystem`
6. Arguments (Optional but recommended): `/path/to/project/root`

---

## 📖 Use Cases

- **Code Auditing**: Recursively scan directories for specific patterns using `search_files`.
- **Automated Refactoring**: Apply precise code changes safely using `edit_file`.
- **Asset Review**: Load and preview media assets directly through the MCP interface.

---

## 💻 CLI Functionality

Check version and verify accessibility:

```bash
./mcp-server-filesystem -version
```

---

*Built with Go for security and stability.*
