# MagicDev MCP Server

A standalone Model Context Protocol (MCP) server for structured requirements engineering, automated documentation generation, and secure enterprise integration with GitLab, Jira, and Confluence.

## Overview

MagicDev provides an **"Idea-to-Asset"** pipeline that transforms raw software ideas into production-ready architectural decision records (MADRs), technical blueprints, Jira tickets, Confluence documentation, and Git-committed artifacts — all driven through an 8-phase Socratic workflow exposed as MCP tools.

### Core Capabilities

| Feature | Description |
|---|---|
| **Socratic Requirements Pipeline** | 8-phase workflow: Evaluate → Ingest Standards → Clarify → Critique → Finalize → Blueprint → Generate Docs → Complete |
| **Hardware-Encrypted Vault** | API tokens are AES-256-GCM encrypted using a hardware-derived machine key (BuntDB vault) |
| **GitLab Integration** | Commits generated documentation, architecture diagrams (D2/SVG), and blueprints directly to your Git repository |
| **Jira Integration** | Automatically creates Jira issues with story point estimates and links generated documentation |
| **Confluence Integration** | Publishes finalized MADRs and technical specs directly to your Confluence space |
| **Semantic Gatekeeper (LLM)** | Optional AI-powered complexity scoring, autonomous standard injection, and enriched requirement analysis |
| **Hot-Reloadable Config** | Configuration changes via `magicdev.yaml` are detected and applied in real-time via `fsnotify` |

---

## Quick Start

### Step 1: Place the Binary

Download the `mcp-server-magicdev` binary for your platform and place it in a directory on your system `PATH`.

#### Linux

```bash
# Move the binary to your local bin directory
mv mcp-server-magicdev ~/.local/bin/mcp-server-magicdev
chmod +x ~/.local/bin/mcp-server-magicdev
```

#### macOS

```bash
# Move the binary to your local bin directory
mv mcp-server-magicdev /usr/local/bin/mcp-server-magicdev
chmod +x /usr/local/bin/mcp-server-magicdev
```

#### Windows (PowerShell)

```powershell
# Create a directory for the binary if it doesn't exist
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\Programs\magicdev"

# Move the binary
Move-Item mcp-server-magicdev.exe "$env:LOCALAPPDATA\Programs\magicdev\mcp-server-magicdev.exe"

# Add to your PATH (current user, persistent)
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$currentPath;$env:LOCALAPPDATA\Programs\magicdev", "User")
```

---

### Step 2: Initialize the Configuration

Run the `init` command to generate the default `magicdev.yaml` configuration file. This must be done **before** the first `configure` or `serve`.

#### Linux / macOS

```bash
mcp-server-magicdev init
```

**Output:**
```
Initializing MagicDev Configuration...
Default configuration successfully generated at /home/youruser/.config/mcp-server-magicdev/magicdev.yaml
```

#### Windows (PowerShell / CMD)

```powershell
mcp-server-magicdev.exe init
```

**Output:**
```
Initializing MagicDev Configuration...
Default configuration successfully generated at C:\Users\YourUser\AppData\Roaming\mcp-server-magicdev\magicdev.yaml
```

> **Configuration file locations by OS:**
>
> | OS | Path |
> |---|---|
> | Linux | `~/.config/mcp-server-magicdev/magicdev.yaml` |
> | macOS | `~/Library/Application Support/mcp-server-magicdev/magicdev.yaml` |
> | Windows | `%APPDATA%\mcp-server-magicdev\magicdev.yaml` |

---

### Step 3: Configure Integrations

Run the interactive `configure` wizard to securely vault your API tokens and set up integrations.

```bash
mcp-server-magicdev configure
```

You will see a numbered menu:

```
=== MagicDev Configuration Menu ===
1. Setup Jira
2. Setup Confluence
3. Setup Gitlab
4. Setup LLM
0. Exit

Select an option:
```

#### Option 1: Setup Jira

Prompts for your Jira email, API token, and instance URL. The email and URL are written to `magicdev.yaml`. The token is stored in the encrypted BuntDB vault.

```
--- Setup Jira ---
Email Address: developer@company.com
Token: ATATT3xFfGF0...
URL (e.g. https://your-domain.atlassian.net): https://mycompany.atlassian.net
Jira configuration saved.
```

#### Option 2: Setup Confluence

Prompts for your Confluence API token and instance URL. The URL is written to `magicdev.yaml`. The token is stored in the encrypted vault.

```
--- Setup Confluence ---
Token: ATATT3xFfGF0...
URL (e.g. https://your-domain.atlassian.net/wiki): https://mycompany.atlassian.net/wiki
Confluence configuration saved.
```

#### Option 3: Setup GitLab

Prompts for your GitLab username and personal access token. The username is written to `magicdev.yaml`. The token is stored in the encrypted vault.

```
--- Setup Gitlab ---
Username: jsmith
Token: glpat-xxxxxxxxxxxxxxxxxxxx
Gitlab configuration saved.
```

#### Option 4: Setup LLM (Semantic Gatekeeper)

Prompts you to select a provider (Gemini, OpenAI, or Claude), then your API key. Once the key is entered, it dynamically fetches the list of available models from the provider's API and presents them for selection:

```
--- Setup LLM ---
1. Gemini
2. OpenAI
3. Claude
0. Cancel
Select Provider: 1

API Key: AIzaSy...
Fetching available models...

Available Models:
1. gemini-2.5-flash
2. gemini-2.5-pro
3. gemini-2.0-flash
...

Select Default Model (number): 1
LLM configuration saved. Default model: gemini-2.5-flash
```

The API key and provider are stored in the encrypted vault. The selected model is written to `magicdev.yaml` under `llm.model` and can be changed at runtime (hot-reloadable).

---

### Step 4: Configure Your IDE

MagicDev communicates over **stdio** using the MCP protocol. You must configure your IDE to launch the binary with the `serve` argument.

---

#### Antigravity (Google DeepMind)

| OS | Configuration File Path |
|---|---|
| Linux | `~/.gemini/antigravity/mcp_config.json` |
| macOS | `~/.gemini/antigravity/mcp_config.json` |
| Windows | `%USERPROFILE%\.gemini\antigravity\mcp_config.json` |

**Linux / macOS:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/home/youruser/.local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicdev\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

> **Tip:** In Antigravity, you can also open this file via: Agent Panel → `...` menu → Manage MCP Servers → View raw config.

---

#### Visual Studio Code (GitHub Copilot / Native MCP)

| OS | User-Level Configuration File Path |
|---|---|
| Linux | `~/.config/Code/User/mcp.json` |
| macOS | `~/Library/Application Support/Code/User/mcp.json` |
| Windows | `%APPDATA%\Code\User\mcp.json` |

You can also place a project-specific configuration at `.vscode/mcp.json` in your project root.

**Linux:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/home/youruser/.local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/usr/local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicdev\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

> **Tip:** In VSCode, open the Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`) and type `MCP: Open User Configuration` to edit this file directly.

---

#### VSCode — Cline Extension

| OS | Configuration File Path |
|---|---|
| Linux | `~/.cline/data/settings/cline_mcp_settings.json` |
| macOS | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| Windows | `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json` |

**Linux:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/home/youruser/.local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/usr/local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicdev\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

> **Tip:** In VSCode, open the Cline sidebar → click the MCP Servers plug icon → Configure → "Edit MCP Settings" to open this file.

---

#### Claude Desktop

| OS | Configuration File Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

**macOS:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/usr/local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicdev\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

---

#### Claude Code (CLI)

Claude Code uses a CLI command to register MCP servers. No manual JSON editing is required.

**Linux:**

```bash
claude mcp add magicdev -s user -- /home/youruser/.local/bin/mcp-server-magicdev serve
```

**macOS:**

```bash
claude mcp add magicdev -s user -- /usr/local/bin/mcp-server-magicdev serve
```

**Windows (PowerShell):**

```powershell
claude mcp add magicdev -s user -- "C:\Users\YourUser\AppData\Local\Programs\magicdev\mcp-server-magicdev.exe" serve
```

> **Verify registration:** Run `claude mcp list` to confirm the server is registered and active.

---

#### Cursor

| OS | Global Configuration File Path |
|---|---|
| Linux | `~/.cursor/mcp.json` |
| macOS | `~/.cursor/mcp.json` |
| Windows | `%USERPROFILE%\.cursor\mcp.json` |

You can also place a project-specific configuration at `.cursor/mcp.json` in your project root.

**Linux:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/home/youruser/.local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/home/youruser"
      }
    }
  }
}
```

**macOS:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/usr/local/bin/mcp-server-magicdev",
      "args": ["serve"],
      "env": {
        "HOME": "/Users/youruser"
      }
    }
  }
}
```

**Windows:**

```json
{
  "mcpServers": {
    "magicdev": {
      "command": "C:\\Users\\YourUser\\AppData\\Local\\Programs\\magicdev\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "USERPROFILE": "C:\\Users\\YourUser"
      }
    }
  }
}
```

> **Tip:** In Cursor, you can also add MCP servers via the UI: Settings → Tools & MCP → "+ Add New MCP Server" → Transport Type: `stdio`, Command: full path to binary, Args: `serve`.

---

## MCP Tools Reference

Once the server is running, the following tools are exposed to your IDE's AI agent:

| Tool | Phase | Description |
|---|---|---|
| `evaluate_idea` | 1 | Initializes a new session for a software idea. Returns a `session_id` used in all subsequent phases. |
| `ingest_standards` | 2 | Fetches and caches architectural standards (Node.js, .NET) relevant to the project's stack. |
| `clarify_requirements` | 3 | Performs Socratic analysis against ingested standards. Returns errors with questions if gaps are detected. |
| `critique_design` | 4 | Vets the proposed architecture against standards for compliance and security. |
| `finalize_requirements` | 5 | Consolidates the vetted design into a "Golden Copy" JSON specification. |
| `blueprint_implementation` | 6 | Generates a technical implementation blueprint with file structure, dependencies, data models, and ADRs. |
| `generate_documents` | 7 | Commits documentation to Git, creates Jira issues, and publishes to Confluence. Renders D2 architecture diagrams as SVG. |
| `complete_design` | 8 | Archives the session and produces a comprehensive handoff summary for the coding agent. |
| `update_config` | — | Programmatically updates a key in `magicdev.yaml` (hot-reloaded via `fsnotify`). |
| `get_internal_logs` | — | Returns the tail of the in-memory server logs for diagnostics. |

### Workflow

The tools enforce a strict sequential dependency chain. Your AI agent must call them in order:

```
evaluate_idea → ingest_standards → clarify_requirements → critique_design
    → finalize_requirements → blueprint_implementation → generate_documents → complete_design
```

Each phase builds upon the state stored from the previous phase. The `session_id` returned by `evaluate_idea` must be passed to every subsequent tool call.

---

## Semantic Gatekeeper (LLM Intelligence)

When an LLM is configured via `mcp-server-magicdev configure` (Option 4), the server activates the **Semantic Gatekeeper** — an AI-powered intelligence layer that enhances the requirements pipeline.

### What It Does

| Capability | Without LLM | With LLM |
|---|---|---|
| **Complexity Scoring** | Not available | Automatic 1-13 story point estimation per feature |
| **Socratic Depth** | Basic gap detection | Deep semantic analysis of requirement completeness |
| **Standard Injection** | Manual standard selection | Autonomous standard matching based on idea analysis |
| **Risk Assessment** | Template-based | Context-aware security and architectural risk scoring |

### Supported Providers

| Provider | SDK | Model Examples |
|---|---|---|
| Google Gemini | `google/generative-ai-go` | `gemini-2.5-flash`, `gemini-2.5-pro` |
| OpenAI | `sashabaranov/go-openai` | `gpt-4o`, `gpt-4o-mini` |
| Anthropic Claude | `anthropics/anthropic-sdk-go` | `claude-sonnet-4-20250514`, `claude-3-5-haiku-latest` |

### Behavior

- **If no LLM is configured:** The server operates normally with all core MCP tools functional. LLM-enhanced features are silently disabled.
- **If the LLM is configured and healthy:** A startup log confirms `Semantic Gatekeeper (LLM) feature enabled and healthy`.
- **If the LLM API key expires or the model becomes unavailable:** The server logs a warning and falls back to default non-LLM behavior. No tools break.
- **Hot-Reload:** Changing the `llm.model` value in `magicdev.yaml` triggers an immediate health check against the new model without restarting the server.

---

## CLI Commands Reference

### `init`

Generates the default `magicdev.yaml` configuration file and exits. Safe to run multiple times — it will not overwrite an existing configuration.

```bash
mcp-server-magicdev init
```

### `configure`

Opens an interactive menu to set up Jira, Confluence, GitLab, and LLM integrations. Non-sensitive values (URLs, usernames, model names) are written to `magicdev.yaml`. Sensitive values (API tokens, keys) are stored in the hardware-encrypted BuntDB vault.

```bash
mcp-server-magicdev configure
```

### `serve`

Starts the MCP server over stdio. This is the command your IDE calls to launch the server. It is not intended to be run interactively by the user.

```bash
mcp-server-magicdev serve
```

### `token list`

Displays all currently stored vault secrets (GitLab, Confluence, Jira, LLM tokens). Useful for verifying that `configure` saved your credentials correctly.

```bash
mcp-server-magicdev token list
```

**Example output:**
```
Stored Tokens / Values:
- gitlab: glpat-xxxxxxxxxxxxxxxxxxxx
- confluence: ATATT3xFfGF0...
- jira: ATATT3xFfGF0...
- llm_token: AIzaSy...
- llm_provider: gemini
- llm_model: (Not Set)
```

### `token reconfigure`

Re-imports tokens from environment variables or prompts interactively for each service. Useful for CI/CD environments or bulk token rotation.

```bash
# Interactive mode
mcp-server-magicdev token reconfigure

# Environment variable mode (non-interactive)
GITLAB_TOKEN=glpat-xxx JIRA_TOKEN=ATATT3x... mcp-server-magicdev token reconfigure
```

**Supported environment variables:**

| Service | Environment Variables (checked in order) |
|---|---|
| GitLab | `GITLAB_TOKEN`, `GITLAB_PERSONAL_ACCESS_TOKEN`, `GITLAB_USER_TOKEN` |
| Confluence | `CONFLUENCE_USER_TOKEN`, `CONFLUENCE_TOKEN`, `CONFLUENCE_API_TOKEN` |
| Jira | `JIRA_USER_TOKEN`, `JIRA_TOKEN`, `JIRA_API_TOKEN` |
| LLM | `LLM_TOKEN`, `LLM_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY` |

### `purge sessions`

Purges all session data from the BuntDB database. Prompts for confirmation before deleting.

```bash
mcp-server-magicdev purge sessions
```

### `purge baselines`

Purges all cached baseline architectural standards from the database. Useful if standards have been updated upstream and you want to force a re-fetch on the next server start.

```bash
mcp-server-magicdev purge baselines
```

---

## Data Storage Locations

| Data | Linux | macOS | Windows |
|---|---|---|---|
| Configuration | `~/.config/mcp-server-magicdev/magicdev.yaml` | `~/Library/Application Support/mcp-server-magicdev/magicdev.yaml` | `%APPDATA%\mcp-server-magicdev\magicdev.yaml` |
| Database (BuntDB) | `~/.cache/mcp-server-magicdev/session.db` | `~/Library/Caches/mcp-server-magicdev/session.db` | `%LOCALAPPDATA%\mcp-server-magicdev\session.db` |
| Server Logs | `stderr` (captured by IDE) | `stderr` (captured by IDE) | `stderr` (captured by IDE) |

---

## Security Architecture

- **Vault-Only Tokens:** API tokens for GitLab, Confluence, Jira, and LLM providers are **never** stored in `magicdev.yaml`. They are encrypted with AES-256-GCM using a hardware-derived machine ID and persisted in BuntDB.
- **Config-Only Settings:** Non-sensitive values (URLs, usernames, model names, feature flags) live in `magicdev.yaml` and are hot-reloadable.
- **Single Instance:** The server enforces a single-instance lock via OS-level file locking. If a stale instance is detected, it is automatically terminated.

---

*Built with Go. Part of the MagicDev Intelligence Suite.*
