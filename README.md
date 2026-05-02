# 🦆 MagicDuck Sub-Server

A high-performance Model Context Protocol (MCP) sub-server providing secure web search and media discovery via DuckDuckGo.

## 🚀 Overview

`mcp-server-duckduckgo` enables AI agents to retrieve real-time information from the web without the need for complex API keys or tracking. It provides a clean, privacy-focused interface for web search, image discovery, and news retrieval.

### 📋 Core Pillars

1.  **Privacy-First Search**: Executes queries via DuckDuckGo, ensuring no user tracking.
2.  **Rich Media Discovery**: Tools for finding images and videos relevant to the current task.
3.  **Real-Time News**: Fetch the latest headlines for up-to-date context.
4.  **Optimized Transport**: Uses a high-performance stdio transport for minimal latency.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`search_web`**: Performs a standard text search and returns a summary of top results.
*   **`search_images`**: Finds images relevant to the query.
*   **`search_news`**: Retrieves the latest news articles.
*   **`get_web_content`**: (If applicable) Extracts markdown content from a specific URL.

### Orchestration with MagicTools (Recommended)
Invoke DuckDuckGo tools via `magictools:call_proxy`:
```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "duckduckgo:search_web",
    "arguments": { "query": "latest Go 1.26 features" }
  }
}
```

---

## ⚙️ Configuration

### 1. Build the Binary
```bash
make build
```

### 2. CLI Options
| Option | Description |
| :--- | :--- |
| `-version` | Print the current version and exit. |

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/absolute/path/to/mcp-server-duckduckgo"
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
    "duckduckgo": {
      "command": "C:/path/to/mcp-server-duckduckgo.exe"
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
    "duckduckgo": {
      "command": "/usr/local/bin/mcp-server-duckduckgo"
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
