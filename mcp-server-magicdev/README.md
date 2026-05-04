# 🛠️ MagicDev Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for secure GitLab and Jira
integration, automating the developer workflow.

## 🚀 Overview

`mcp-server-magicdev` bridges the gap between your local development environment and your
enterprise collaboration tools. It provides secure, hardware-encrypted access to GitLab for
repository management and Jira (Atlassian) for issue tracking.

### 📋 Core Pillars

* **Hardware Encryption**: Credentials (API tokens, SSH keys) are AES-256-GCM encrypted using a hardware-derived key.
* **GitLab Integration**: Automates branch creation, merge requests, and pipeline monitoring.
* **Jira Connectivity**: Create, update, and transition Jira issues directly from your IDE.
* **Secure Vault**: Provides a CLI-based configuration wizard to handle sensitive tokens safely.

---

## 🛠️ Usage & Functionality

### Specialized Tools

* **`jira_get_issue`**: Retrieves full details of a Jira ticket.
* **`gitlab_create_mr`**: Automatically generates a GitLab Merge Request for the current branch.
* **`gitlab_list_projects`**: Searches for projects within your GitLab instance.
* **`setup_dev_env`**: (If applicable) Configures local Git settings and SSH keys.

### Orchestration with MagicTools (Recommended)

Invoke MagicDev tools via `magictools:call_proxy`:

```json
{
  "name": "magictools:call_proxy",
  "arguments": {
    "urn": "magicdev:jira_get_issue",
    "arguments": { "issue_key": "PROJ-123" }
  }
}
```

---

## 📦 Installation & Setup

### 1. Build & Initial Configuration

```bash
make build
./dist/mcp-server-magicdev configure
```

**MANDATORY:** You must run the `configure` command first to securely vault your GitLab and Jira
credentials.

### 2. MagicTools Orchestrator Configuration (Recommended)

Add this to your `~/.config/mcp-server-magictools/servers.yaml` to run MagicDev as an orchestrated
sub-server:

```yaml
- name: magicdev
  command: /absolute/path/to/mcp-server-magicdev
  args:
    - serve
  env:
    HOME: /absolute/path/to/home
  memory_limit_mb: 2048
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
    "magicdev": {
      "command": "/absolute/path/to/mcp-server-magicdev",
      "args": ["serve"],
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
    "magicdev": {
      "command": "C:\\path\\to\\mcp-server-magicdev.exe",
      "args": ["serve"],
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
    "magicdev": {
      "command": "/absolute/path/to/mcp-server-magicdev",
      "args": ["serve"],
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
    "magicdev": {
      "command": "C:\\path\\to\\mcp-server-magicdev.exe",
      "args": ["serve"],
      "env": {
        "HOME": "C:\\Users\\YourName"
      }
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
