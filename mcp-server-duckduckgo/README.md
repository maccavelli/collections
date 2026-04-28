# 🦆 DuckDuckGo Search MCP Server

A high-performance Model Context Protocol (MCP) server providing secure, anonymous, and comprehensive web search
capabilities for AI agents.

## 🚀 Overview

The DuckDuckGo Search server empowers AI-driven agents and developers to integrate real-world awareness into their
workflows without compromising privacy or security. It abstracts the complexities of search parsing and provides
structured, token-efficient results.

### 📋 Core Pillars

1. **Web & News Search**: Real-time retrieval of relevant web pages and latest news articles.
2. **Media Discovery**: Search for high-quality images and video content with structured metadata.
3. **Academic & Reference Search**: Specialized search for books and educational materials.
4. **Token Efficiency**: Automatic truncation and intelligent summarization of results.

---

## 🛠️ Tools

### `search_web`

Perform a broad web search. Returns structured titles, snippets, and URLs.

- **Parameters**: `query` (string), `max_results` (int, optional), `format` (string, optional: `hybrid`, `json`,
  `markdown`)

### `search_news`

Retrieve latest news articles related to the query.

- **Parameters**: `query` (string), `max_results` (int, optional)

### `search_images`

Find images related to the query. Returns structured JSON with image URLs and metadata.

- **Parameters**: `query` (string), `max_results` (int, optional)

### `search_videos`

Discover video content from across the web.

- **Parameters**: `query` (string), `max_results` (int, optional)

### `search_books`

Search for published book titles and metadata.

- **Parameters**: `query` (string), `max_results` (int, optional)

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-duckduckgo main.go
```

### 2. Configure for IDEs

#### **Antigravity**

```yaml
mcpServers:
  duckduckgo:
    command: "/absolute/path/to/dist/mcp-server-duckduckgo"
```

#### **VS Code (MCP Extension / Cline)**

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/absolute/path/to/dist/mcp-server-duckduckgo",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

#### **Cursor IDE**

1. Open **Settings** -> **Features** -> **MCP**.
2. **+ Add New MCP Server**.
3. Name: `DuckDuckGo`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-duckduckgo`

---

## 📖 Use Cases

- **Up-to-Date Research**: Use `search_news` to stay current with breaking tech news.
- **Code Asset Discovery**: Use `search_images` for UI inspiration or finding assets.
- **Reference Verification**: Use `search_web` to cross-verify documentation or claims.

---

## 💻 CLI Functionality

Supports checking version:

```bash
./mcp-server-duckduckgo -version
```

---

*Built with Go for speed and privacy.*
