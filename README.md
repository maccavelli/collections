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

-   **High-Concurrency Multi-Provider Resilience**: Implements a high-concurrency search engine that queries multiple providers (DuckDuckGo, Google, Bing) in parallel using Go channels.
    -   **Parallel Querying**: All available providers are queried simultaneously to minimize latency.
    -   **Early Completion**: The engine returns results as soon as the `max_results` threshold is met, canceling pending requests to save resources.
    -   **Intelligent Fallback**: Seamlessly falls back to secondary providers if primary sources are unavailable or return insufficient results.
-   **Single-flight Request Protection**: Utilizes `golang.org/x/sync/singleflight` to prevent redundant network requests for the same query, ensuring high efficiency even under heavy concurrent load.
-   **Structured Result Engine**: Converts raw HTML/JSON search results from various sources into a clean, uniform schema optimized for AI consumption.

### Why it exists (Rationale)

LLMs are often limited by their training data cutoff. The DuckDuckGo server provides a bridge to current events and public domain data while:

-   **Ensuring Privacy**: Standard search engines often track user queries. This server provides a proxy that protects user anonymity.
-   **Optimizing Context**: Raw search results are often verbose and noisy. This server cleans and truncates data to ensure only the highest signal reaches the LLM.

## Tools

### General Search
-   `ddg_search_web(query, [max_results], [format])`: Perform a broad web search. Returns structured titles, snippets, and URLs.
-   `ddg_search_news(query, [max_results], [format])`: Retrieve latest news articles related to the query.
-   `ddg_search_books(query, [max_results], [format])`: Search for published book titles and metadata.

### Media Search
-   `ddg_search_images(query, [max_results])`: Find images related to the query. Returns **Structured JSON 2.0** with source URLs, media URLs, and thumbnails.
-   `ddg_search_videos(query, [max_results])`: Discover video content from across the web. Returns **Structured JSON 2.0** with duration and publisher metadata.

#### **Format Options (General Search Only)**
- `hybrid` (Default): Returns a JSON envelope with machine-readable `metadata` and a `results_md` field for AI ingestion.
- `json`: Returns structured data as pure JSON.
- `markdown`: Returns only the formatted Markdown string.

> [!NOTE]
> Media search tools (Images and Videos) exclusively use **Structured JSON 2.0** for optimal metadata delivery and to avoid rendering issues associated with Markdown embedding.

### System Diagnostics
-   `get_internal_logs(max_lines)`: Retrieves recent server activity logs for monitoring and debugging.

## Installation

### 1. Build the Binary

Ensure you have Go installed, then build the binary:

```bash
go build -o dist/mcp-server-duckduckgo main.go
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
    "duckduckgo": {
      "command": "C:\\path\\to\\mcp-server-duckduckgo.exe",
      "args": [],
      "env": {
        "PATH": "C:\\Program Files\\Go\\bin;C:\\Windows\\system32"
      }
    }
  }
}
```

#### **Linux / MacOS**
```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/usr/local/bin/mcp-server-duckduckgo",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

## Use Cases

- **Up-to-Date Research**: Use `ddg_search_news` to stay current on rapidly changing technical fields or news events.
- **Code Asset Discovery**: Use `ddg_search_images` to find UI icons or design inspirations while building applications.
- **Reference Verification**: Cross-verify claims or technical documentation using `ddg_search_web`.

---

Created in Go for reliability and speed.
