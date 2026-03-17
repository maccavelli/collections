# DuckDuckGo MCP Server (Go)

A high-performance Model Context Protocol (MCP) server that provides comprehensive search capabilities.

## 🎯 What it is for

The `duckduckgo` MCP server enables AI agents to stay grounded in real-world data by providing dedicated tools for Web,
News, Images, Videos, and Books. It is designed for speed, resilience, and rich metadata extraction.

## ⚙️ How it works

- **Native Protocol**: Communicates via `JSON-RPC 2.0` over `stdio` transport as per the MCP specification.
- **Scraping Architecture**: Intelligently extracts a `vqd` token from DuckDuckGo to authenticate requests to internal
  JSON endpoints used for News, Images, and Videos.
- **Anna's Archive Integration**: Uses a high-performance concurrent scraper to query multiple book archive mirrors.

## 🧠 Why it works

- **Golang Efficiency**: Written in Go to ensure minimal memory overhead and near-instant execution, which is critical
  for agentic tool loops.
- **Mirror Resilience**: The book search queries multiple TLDs (e.g., `.gd`, `.gl`, `.pk`) in parallel. The first
  successful response is returned immediately.
- **Rich Metadata Extraction**: Unlike simple link scrapers, this server extracts thumbnails, durations, authors, and
  file formats, providing the AI with superior context.

## � Installation Instructions

1. **Pre-built Binaries**: You can simply copy the pre-built binary for your platform from the `bin/` directory to any
   location on your system.
2. **Deploy**: Update your MCP host configuration with the absolute path to the binary.

   *Example*: If you place the Linux binary in `~/.local/bin`, your configuration path would be:
   `/home/username/.local/bin/mcp-server-duckduckgo-linux-amd64`

3. **Build from Source** (Optional):

   ```bash
   go build -o mcp-server-duckduckgo
   ```

4. **Verify**:

   ```bash
   ./mcp-server-duckduckgo --version
   ```

## �🚀 Usage Instructions

Register the server in your Antigravity or Claude Desktop `mcp_config.json`:

```json
{
  "mcpServers": {
    "duckduckgo": {
      "command": "/path/to/mcp-server-duckduckgo-linux-amd64",
      "args": [],
      "env": {}
    }
  }
}
```

Available tools:

- `ddg_search_web`: High-quality web search snippets.
- `ddg_search_news`: Recent news articles with timestamps.
- `ddg_search_images`: Visual content discovery.
- `ddg_search_videos`: Video metadata and links.
- `ddg_search_books`: Deep search for books via Anna's Archive.

## 💡 Detailed Guidance

- **Rate Limiting**: While resilient, excessive rapid-fire requests may lead to temporary blocks by DuckDuckGo.
- **Book Search Latency**: Book results may take 2-4 seconds as multiple mirrors are queried across regions.
- **Metadata Quality**: Image and video metadata depends on DuckDuckGo's internal extraction; results may vary.

## ⚖️ License

MIT

---
*Built with Go for performance and stability.*
