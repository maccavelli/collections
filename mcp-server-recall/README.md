# MCP Generic Recall Server

A Model Context Protocol (MCP) server designed to provide AI agents with long-term, structured "memory" for
architectural decisions, project context, and lessons learned. It ensures that critical design choices and institutional
knowledge persist across various chat sessions, environments, and tool restarts.

## What is it for?

AI context windows are ephemeral. When you start a new session or switch projects, the AI loses the nuances of your
previous architectural decisions. The Recall Server bridges this gap by providing a persistent, searchable key-value
store that an AI can use to "remember" why something was built a certain way and "recall" it later when needed.

## Key Features

- **Structured Knowledge**: Each memory is stored as a Record with `Content`, `Tags`, `CreatedAt`, and `UpdatedAt`.
- **Tagging Support**: Categorize memories (e.g., `arch`, `context`, `lesson-learned`) for targeted retrieval.
- **Searchable Index**: Prefix and keyword search across all stored keys, values, and tags.
- **Auto-Migration**: Seamlessly upgrades legacy string-only data to structured records without data loss.
- **Configurable Storage**: Use `MCP_RECALL_DB_PATH` to specify a custom database directory.
- **High-Performance Persistence**: Built on BadgerDB (K/V store) for atomic updates and fast iteration.
- **Resilient**: Graceful shutdown handling ensures data integrity even on process interrupts.

## How it Works

The server implements the Model Context Protocol (MCP) over a standard input/output (stdio) transport. It exposes a set
of tools that allow any MCP-compatible client (like an IDE or an AI assistant) to interact with the underlying BadgerDB
database.

### Internal Logic

- **MemoryStore**: A Go wrapper around BadgerDB that handles atomic updates and JSON record serialization.
- **Search**: A concurrent-safe iterator-based search that scans keys, content, and tags for specific patterns.
- **Graceful Shutdown**: Intercepts `SIGINT` and `SIGTERM` to safely close the database and flush all writes.

## Installation

### 1. Build the Binary

Ensure you have Go installed, then build the binary:

```bash
go build -o dist/mcp-server-recall main.go
```

The compiled binary will be located in the `dist` directory.

### 2. Configuration for AI Agents (Antigravity, Claude, Cline)

To use this server with an MCP-compatible agent, add it to your `mcpServers` configuration file.

#### **Windows**
> [!IMPORTANT]
> On Windows, you **MUST** include the `.exe` extension in the command path for the agent to correctly invoke the binary.

```json
{
  "mcpServers": {
    "recall": {
      "command": "C:\\path\\to\\mcp-server-recall.exe",
      "args": [],
      "env": {
        "PATH": "C:\\Program Files\\Go\\bin;C:\\Windows\\system32",
        "MCP_RECALL_DB_PATH": "C:\\Users\\User\\.mcp_recall"
      }
    }
  }
}
```

#### **Linux / MacOS**
```json
{
  "mcpServers": {
    "recall": {
      "command": "/usr/local/bin/mcp-server-recall",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin",
        "MCP_RECALL_DB_PATH": "/home/user/.mcp_recall"
      }
    }
  }
}
```

## Configuration

The server supports the following environment variables:

- `MCP_RECALL_DB_PATH`: The directory where the Badger database files are stored.
- `PATH`: Should include the path to the Go binary if running in a development environment.

## Tools

The server exposes the following MCP tools:

1.  `remember`: Saves or updates a memory with optional categorization.
    - `key`: A unique identifier (e.g., `api-auth-pattern`).
    - `value`: The content to store.
    - `tags`: (Optional) An array of strings to categorize the memory (e.g. `["architecture", "security"]`).
2.  `recall`: Retrieves a structured memory and its metadata by its key.
    - `key`: The identifier to look up.
3.  `search_memories`: Searches for a query string in keys, values, and tags.
    - `query`: The search term.
    - `tag`: (Optional) Filter results to a specific tag.
4.  `list_memories`: Returns a complete index of all memory keys and their associated tags.
    - Useful for AI agents to "browse" existing knowledge before performing a specific recall.
5.  `forget`: Removes a memory.
    - `key`: The identifier to delete.

## Example Use Cases

### Scenario: Categorizing Architectural Patterns

**Tool Call:**
```json
{
  "tool": "remember",
  "arguments": {
    "key": "microservice-error-middleware",
    "value": "Use 'ErrorInterceptor' v2.1.0 for consistent JSON responses.",
    "tags": ["architecture", "standardization"]
  }
}
```

### Scenario: Searching by Tag

**Tool Call:**
```json
{
  "tool": "search_memories",
  "arguments": {
    "tag": "architecture"
  }
}
```

**Result:**
The AI retrieves a list of all architectural decisions, providing a quick summary of the project's high-level design in
one call.

---

*Built with Go for performance and reliability.*
