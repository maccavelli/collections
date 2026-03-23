# DuckDuckGo Search Server

A high-performance MCP server providing secure, anonymous, and comprehensive web search capabilities, including news, media, and academic resources.

## Overview

The DuckDuckGo Search server empowers AI-driven agents and developers to integrate real-world awareness into their workflows without compromising privacy or security. It abstracts the complexities of search parsing and provides structured, token-efficient results.

### What it does (Core Pillars)

1.  **Web & News Search**: Real-time retrieval of relevant web pages and latest news articles.
2.  **Media Discovery**: Search for high-quality images and video content with structured metadata.
3.  **Academic & Reference Search**: Specialized search for books and educational materials.
4.  **Token Efficiency**: Automatic truncation and intelligent summarization of results to maximize LLM context window efficiency.

### How it works (Architecture)

Built in Go for speed and concurrent processing, the server utilizes a distributed handler architecture:

-   **Modular Handler System**: Separate packages for `search` (web, news, books) and `media` (images, videos) allow for specialized processing and scaling.
-   **Structured Result Engine**: Converts raw HTML/JSON search results into a clean, uniform schema optimized for AI consumption.
-   **Concurrent Retrieval**: Uses Go's concurrency primitives to perform multi-source searches with minimal latency.
-   **Privacy-Native**: Uses DuckDuckGo's anonymous endpoints to ensure no user-identifiable data is tracked or transmitted.

### Why it exists (Rationale)

LLMs are often limited by their training data cutoff. The DuckDuckGo server provides a bridge to current events and public domain data while:

-   **Ensuring Privacy**: Standard search engines often track user queries. This server provides a proxy that protects user anonymity.
-   **Optimizing Context**: Raw search results are often verbose and noisy. This server cleans and truncates data to ensure only the highest signal reaches the LLM.

## Tools

### General Search
-   `ddg_search_web(query, [max_results])`: Perform a broad web search. Returns structured titles, snippets, and URLs.
-   `ddg_search_news(query, [max_results])`: Retrieve latest news articles related to the query.
-   `ddg_search_books(query, [max_results])`: Search for published book titles and metadata.

### Media Search
-   `ddg_search_images(query, [max_results])`: Find images related to the query, providing source URLs and titles.
-   `ddg_search_videos(query, [max_results])`: Discover video content from across the web.

### System Diagnostics
-   `get_internal_logs(max_lines)`: Retrieves recent server activity logs for monitoring and debugging.

## Installation

### 1. Build the Binary
```bash
make build
```
The compiled binary will be located in the `dist` directory.

### 2. Configure for IDEs

#### **Antigravity**
Add the server to your `mcpServers` configuration:
```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/usr/local/bin/mcp-server-duckduckgo",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin"
      }
    }
  }
}
```

#### **VS Code (MCP Extension)**
If using an MCP-compatible VS Code extension (like Claude Dev or Cline):
1.  Navigate to the setting/config file for the extension.
2.  Add the configuration entry:
```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/path/to/dist/mcp-server-duckduckgo",
      "args": []
    }
  }
}
```

## Use Cases

-   **Up-to-Date Research**: Use `ddg_search_news` to stay current on rapidly changing technical fields or news events.
-   **Code Asset Discovery**: Use `ddg_search_images` to find UI icons or design inspirations while building applications.
-   **Reference Verification**: Cross-verify claims or technical documentation using `ddg_search_web`.

---

Created in Go for reliability and speed.
