# 🛠️ MagicDev Sub-Server

A high-performance Model Context Protocol (MCP) sub-server for secure GitLab and Jira integration, automating the developer workflow.

## 🚀 Overview

`mcp-server-magicdev` bridges the gap between your local development environment and your enterprise collaboration tools. It provides secure, hardware-encrypted access to GitLab for repository management and Jira (Atlassian) for issue tracking.

### 📋 Core Pillars

1.  **Hardware Encryption**: Credentials (API tokens, SSH keys) are AES-256-GCM encrypted using a hardware-derived key.
2.  **GitLab Integration**: Automates branch creation, merge requests, and pipeline monitoring.
3.  **Jira Connectivity**: Create, update, and transition Jira issues directly from your IDE.
4.  **Secure Vault**: Provides a CLI-based configuration wizard to handle sensitive tokens safely.

---

## 🛠️ Usage & Functionality

### Specialized Tools

*   **`jira_get_issue`**: Retrieves full details of a Jira ticket.
*   **`gitlab_create_mr`**: Automatically generates a GitLab Merge Request for the current branch.
*   **`gitlab_list_projects`**: Searches for projects within your GitLab instance.
*   **`setup_dev_env`**: (If applicable) Configures local Git settings and SSH keys.

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

## ⚙️ Initial Configuration

You MUST use the `configure` CLI command to securely vault your credentials before the server can operate.

### 1. Build the Binary
```bash
make build
```

### 2. Run the Configuration Wizard
```bash
./dist/mcp-server-magicdev configure
```
**What this does:**
*   **Atlassian Setup**: Prompts for your Site URL and API Token.
*   **GitLab Setup**: Prompts for your Personal Access Token.
*   **SSH Setup**: Prompts for your private key path (e.g., `~/.ssh/id_rsa`).
*   **Encryption**: Encrypts and saves the configuration to your platform's standard config directory.

---

## 🖥️ IDE Configuration Examples (Standalone)

### 🌌 Antigravity
**Path:** `~/.gemini/mcp_config.json`
```json
{
  "mcpServers": {
    "magicdev": {
      "command": "/absolute/path/to/mcp-server-magicdev",
      "args": ["serve"]
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
    "magicdev": {
      "command": "C:/path/to/mcp-server-magicdev.exe",
      "args": ["serve"]
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
    "magicdev": {
      "command": "/usr/local/bin/mcp-server-magicdev",
      "args": ["serve"]
    }
  }
}
```

---

*Part of the MagicTools Intelligence Suite. Built with Go.*
