# Recall Memory Server

A high-performance Model Context Protocol (MCP) server providing AI agents with long-term, structured "memory" for architectural decisions, project context, and lessons learned. It ensures that critical design choices and institutional knowledge persist across sessions and environments.

## Overview

The Recall Server bridges the gap between ephemeral AI context windows and persistent project requirements. It provides a local-first, searchable key-value store that empowers agents to "remember" why decisions were made and "recall" them precisely when needed.

### What it does (Core Pillars)

1.  **Knowledge Persistence**: Long-term storage for architectural patterns, environment-specific context, and session-derived lessons.
2.  **Dual-Layer Organization**: Support for primary **Categories** (stable classification) and multi-value **Tags** (cross-referencing) for flexible discovery.
3.  **Intelligent Consolidation**: Automated redundancy detection that identifies and merges overlapping memories into concise, high-density entries.
4.  **Blazing-Fast Retrieval**: High-concurrency search and ranking engine that surfaces relevant context in milliseconds.

### How it works (Architecture)

Built in Go for speed and transactional integrity, the server maintains a hardened storage layer:

-   **BadgerDB-Powered Resilience**: Utilizes a local-first K/V store for atomic updates and fast iteration, ensuring data safety even on process interrupts.
-   **Concurrency Mandate**: Every heavy operation—from fuzzy scoring during search to pairwise similarity checks during consolidation—is parallelized across CPU cores using worker pools.
-   **Intelligent Ranking Engine**: Implements a multi-field fuzzy matching algorithm that scores matches across keys, content, categories, and tags.
-   **Token Economy**: Automatically summarizes or truncates large records during search results to maximize the efficiency of the AI context window.

### Why it exists (Rationale)

-   **Bridging Context Drift**: AI agents often lose track of "the why" behind a project when sessions reset. Recall ensures that "yesterday's epiphany" is "today's requirement."
-   **Noise Reduction**: As projects grow, redundant memories can clutter results. The `consolidate_memories` tool ensures your knowledge base remains high-signal and low-noise.
-   **Privacy-First Design**: Since architectural data is often sensitive, this server runs entirely locally, ensuring that institutional knowledge never leaves your infrastructure.

## Tools

The server exposes a comprehensive suite of tools optimized for autonomous agent interaction:

### Knowledge Ingestion & Retrieval
-   `remember(key, value, [category], [tags])`: **Knowledge Ingestion**. Saves or updates an atomic bit of context. Use `category` for primary classification and `tags` for fluid cross-referencing.
-   `recall(key)`: **Direct Access**. Retrieves a structured memory and its full metadata by its unique key. 
-   `recall_recent([count])`: **Session Recovery**. Retrieves the most recently modified memories. Perfect for resuming context after an IDE or server restart.
-   `list_memories()`: **Knowledge Index**. Returns a complete list of all stored keys and their metadata summaries for browsing.
-   `list_categories()`: **Structure Discovery**. Returns a unique list of all categories currently stored in the database with entry counts.

### Search & Discovery
-   `search_memories(query, [tag], [limit])`: **Fuzzy Discovery**. Performs a ranked search across keys, values, and tags. Returns the highest relevance matches.
-   `consolidate_memories([similarity_threshold], [dry_run])`: **Database Optimization**. Algorithmically identifies redundant or overlapping memories and merges them into concise summaries using Go-based similarity logic.

### Database Operations
-   `forget(key)`: **Memory Erasure**. Permanently removes a specific memory and its associated indices.
-   `memory_status()`: **System Health**. Provides database usage statistics, including total entry count and estimated storage size.
-   `clear_all_memories()`: **Total Reset**. Permanently wipes all memories from the store. Use with extreme caution.

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

## Use Cases

- **Architectural Preservation**: Use `remember` with the `architecture` category to log why a specific library or pattern was chosen.
- **Workflow Memoization**: Log complex configuration steps or command sequences that are hard to remember but frequently used.
- **Bug Remediation History**: Track the resolution steps of subtle bugs categorized by `bug-fix` to prevent regressions.

---

*Built with Go for performance and reliability.*
