# DuckDuckGo MCP Server (Go)

A high-performance Model Context Protocol (MCP) server written in Go that provides
comprehensive search capabilities using DuckDuckGo and Anna's Archive.

This server is designed for speed, resilience, and rich metadata extraction,
making it an ideal companion for AI agents that need to stay grounded in
real-world data.

## ✨ Features

- **🌐 Multi-Modal Search**: Dedicated tools for Web, News, Images, and Videos
  via DuckDuckGo.
- **📚 Resilient Book Search**: High-performance book discovery via Anna's
  Archive mirrors, utilizing concurrent requests across multiple TLDs (e.g.,
  `.gd`, `.gl`, `.pk`) for maximum speed and uptime.
- **💎 Rich Metadata**: Goes beyond simple links. Extracts snippets, thumbnails,
  durations, authors, and file formats where available.
- **⚡ Performance**: Built in Go for minimal overhead and lightning-fast
  execution.
- **🛡️ Resilience**: Implements intelligent scraping of DuckDuckGo's HTML and
  JSON endpoints to avoid common API limitations.

## 🚀 Installation & Setup

To register this server in your Antigravity `mcp_config.json`, add the following
entry to the `mcpServers` object, changing the -os- to the os version of the build:

```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/absolute/path/to/mcp-server-duckduckgo-os-amd64",
      "args": [],
      "env": {}
    }
  }
}
```

> [!TIP]
> You can verify the installation by running the binary with the `--version`
> flag:
> `./mcp-server-duckduckgo-linux-amd64 --version`

---

## 🛠️ Tools Provided

### 1. `ddg_search_web`

Performs a high-quality web search using the DuckDuckGo HTML endpoint.

- **Parameters**:
  - `query` (string, required): Search keywords.
  - `max_results` (number, default: 5): Results to return.
- **Returns**: Title, URL, and snippet description.

### 2. `ddg_search_news`

Searches for recent news articles.

- **Parameters**: `query`, `max_results`.
- **Returns**: Title, URL, excerpt, source name, publication date, and thumbnail
  URL.

### 3. `ddg_search_images`

Discovery of image content.

- **Parameters**: `query`, `max_results`.
- **Returns**: Title, source URL, direct image URL, and thumbnail.

### 4. `ddg_search_videos`

Discovery of video content.

- **Parameters**: `query`, `max_results`.
- **Returns**: Title, URL, description, duration, and publisher.

### 5. `ddg_search_books`

Deep search for books and documents via Anna's Archive.

- **Parameters**: `query`, `max_results`.
- **Returns**: Title, direct link, author, description, and a `metadata` object
  containing size, format, year, language, and category.

---

## 🏗️ Technical Architecture

- **DuckDuckGo Engine**: The server intelligently extracts a `vqd` (Verification
  Query Description) token from the main site to authenticate requests to
  internal JSON endpoints used for News, Images, and Videos. Web results are
  scraped from the reliable HTML-only endpoint.
- **Concurrent Book Search**: The Book tool queries multiple Anna's Archive
  mirrors simultaneously. The first successful response is returned immediately,
  and pending requests are cancelled to ensure the lowest possible latency.
- **Standard Protocol**: Communicates over `JSON-RPC 2.0` via `stdio` transport
  as per the MCP specification.
