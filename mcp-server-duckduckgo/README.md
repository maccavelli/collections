# DuckDuckGo MCP Server (Go)

A high-performance Model Context Protocol (MCP) server that provides
comprehensive, structured search capabilities for AI agents.

## 🎯 Purpose and Capabilities

The `mcp-server-duckduckgo` enables AI agents to stay grounded in real-world,
real-time data by providing dedicated, high-performance tools for **Web**,
**News**, **Images**, **Videos**, and **Books**. It is designed for speed, low
memory overhead, and rich metadata extraction.

### Core Features

- **Comprehensive Search Types**: Dedicated tools for five distinct data
  domains.
- **Rich Metadata Extraction**: Provides more than just links — includes
  thumbnails, video durations, news sources, publication dates, and book
  details (authors, file formats, mirrors).
- **Concurrent Book Search**: Queries multiple Anna's Archive mirrors
  (.gd, .gl, .pk) simultaneously for maximum resilience and speed.
- **Optimized Data Model**: Uses a simplified `SearchResponse` structure to
  reduce token overhead for AI agents.
- **Language**: Built with **Go 1.26.1+** for near-instant execution and
  minimal system resource footprint.

## ⚙️ How it Works

1. **Protocol**: Implements the Model Context Protocol using `stdio` transport,
   communicating via `JSON-RPC 2.0`.
2. **Authentication**: Automatically extracts `vqd` (Verification Query Data)
   tokens from DuckDuckGo to authenticate requests to internal JSON-based
   search endpoints.
3. **Scraping Architecture**:
   - **Web Search**: Uses a high-quality HTML scraper with `goquery` for
     reliable snippet extraction.
   - **JSON Endpoints**: Interacts directly with optimized internal DDG
     endpoints for News, Images, and Videos.
   - **Concurrent Mirroring**: The Book Search uses a `cancel-on-first-success`
     goroutine pattern to query multiple book mirrors concurrently.

## 🧠 Why Go?

- **Zero Runtime Dependencies**: Compiles to a single, statically linked binary.
- **Concurrency**: Goroutines handle parallel scraping of book mirrors without
  complex state management.
- **Performance**: Near-zero startup time and minimal memory consumption
  (typically <20MB RSS) make it ideal for frequent agentic tool invocations.

## 🛠️ Installation

### 1. Requirements

- **Go 1.26.1+** (if building from source)
- An MCP host environment (e.g., Antigravity, Claude Desktop, or NotebookLM)

### 2. Deployment

You can use the pre-built binaries in Releases, or build your own.

**Pre-built Binaries**:
If you use the included Linux binary:
`./dist/mcp-server-duckduckgo-linux-amd64`

**Build from Source**:

```bash
make linux  # Compiles for Linux AMD64
# or
make build  # Compiles for your current OS/Arch
```

### 3. Verification

```bash
./dist/mcp-server-duckduckgo-linux-amd64 --version
```

## 🚀 Usage Instructions

Register the server in your MCP host configuration (`mcp_config.json`):

```json
{
  "mcpServers": {
    "ddg-search": {
      "command": "/path/to/dist/mcp-server-duckduckgo-linux-amd64",
      "args": [],
      "env": {}
    }
  }
}
```

### Available Tools

| Tool | Description | Args (Default) |
| :--- | :--- | :--- |
| `ddg_search_web` | Web search results. | query, max_results (5) |
| `ddg_search_news` | Recent news articles. | query, max_results (5) |
| `ddg_search_images` | Visual image search. | query, max_results (5) |
| `ddg_search_videos` | Video search & metadata. | query, max_results (5) |
| `ddg_search_books` | Concurrent book search. | query, max_results (5) |

## 🛠️ Development

This project uses a standard `Makefile` for common tasks:

- `make build`: Build for the current platform.
- `make build-all`: Cross-compile for Linux, macOS (Intel/M1), and Windows.
- `make test`: Run unit tests for search logic.
- `make fmt`: Format source code.
- `make clean`: Remove build artifacts.

## 💡 Best Practices

- **Rate Limiting**: DuckDuckGo may temporarily block IPs after extremely
  rapid-fire requests. Use reasonable intervals between intensive searches.
- **Book Search Latency**: Book results may take 2-4 seconds as mirrors are
  queried across different regions.
- **Large Results**: Increasing `max_results` beyond 10 for images/videos can
  lead to significantly larger payloads.

## ⚖️ License

MIT

___________________________________________________________________________

Created in Go for performance and efficiency.
