# 🦆 MagicDuck Sub-Server

A high-performance Model Context Protocol (MCP) sub-server providing secure web search and
media discovery via DuckDuckGo.

## 🚀 Overview

`mcp-server-duckduckgo` enables AI agents to retrieve real-time information from the web without the
need for complex API keys or tracking. It provides a clean, privacy-focused interface for web search,
image discovery, and news retrieval.

### 📋 Core Pillars

* **Privacy-First Search**: Executes queries via DuckDuckGo, ensuring no user tracking.
* **Rich Media Discovery**: Tools for finding images and videos relevant to the current task.
* **Real-Time News**: Fetch the latest headlines for up-to-date context.
* **Optimized Transport**: Uses a high-performance stdio transport for minimal latency.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`search_web`**: Performs a standard text search and returns a summary of top results.
* **`search_images`**: Finds images relevant to the query.
* **`search_news`**: Retrieves the latest news articles.
* **`get_web_content`**: (If applicable) Extracts markdown content from a specific URL.

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

## 📦 Installation & Setup

### 1. Build the Binary

```bash
make build
```

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run DuckDuckGo as an orchestrated sub-server:

```yaml
- name: ddg-search
  command: /absolute/path/to/mcp-server-duckduckgo
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
    "duckduckgo": {
      "command": "/absolute/path/to/mcp-server-duckduckgo",
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
    "duckduckgo": {
      "command": "C:\\path\\to\\mcp-server-duckduckgo.exe",
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
    "duckduckgo": {
      "command": "/absolute/path/to/mcp-server-duckduckgo",
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
    "duckduckgo": {
      "command": "C:\\path\\to\\mcp-server-duckduckgo.exe",
      "env": {
        "HOME": "C:\\Users\\YourName"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
