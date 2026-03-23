# MagicSkills Server

A specialized MCP server for high-density knowledge retrieval, skill discovery, and automated workflow bootstrapping.

## Overview

The MagicSkills server manages a sophisticated repository of expert-level "skills"—complex, multi-step procedures and domain knowledge. It is designed to act as a long-term memory and procedural accelerator for developers and AI agents.

### What it does (Core Pillars)

1.  **Skill Discovery & Matching**: Intelligent searching for relevant procedures using semantic matching (BM25) and tag filtering.
2.  **Density & Compression**: Refines large, verbose documentation into "Dense Summaries" optimized for LLM context windows.
3.  **Workflow Bootstrapping**: Automatically extracts checklists and validation steps from skill definitions.
4.  **Host Verification**: Audits and validates the necessary environment dependencies (binaries, permissions) required for a specific skill.

### How it works (Architecture)

Built in Go for maximum performance and memory efficiency, MagicSkills follows a robust, repository-based architecture:

-   **Multi-Root Knowledge Index**: Can index and serve skills from multiple local or remote directories, allowing for partitioned knowledge bases (e.g., internal corp vs. public community).
-   **Advanced Scoring Engine**: Uses a BM25-based similarity engine to find the most relevant skill for a given user intent, even if keywords aren't exact.
-   **Structured Extraction**: Tools like `magicskills_bootstrap` use AST-like parsing to pull actionable task lists directly from YAML/Markdown skill definitions.
-   **Dynamic Resource Loading**: Skills are treated as dynamic resources that can be updated in real-time without server restarts.

### Why it exists (Rationale)

LLMs are excellent at reasoning but struggle to remember company-specific workflows, complex internal setup guides, or precise semantic patterns. MagicSkills provides:

-   **Expertise Persistence**: Captures senior developer knowledge into a machine-readable format.
-   **Automated Assurance**: Reduces "hallucination" by providing the LLM with direct, validated checklists.
-   **Rapid Onboarding**: New developers or agents can immediately "bootstrap" into a complex project with all dependencies verified.

## Tools

### Skill Navigation
-   `magicskills_list()`: Provides a comprehensive list of all skills available in the current index.
-   `magicskills_match(intent)`: Automatically finds the best-matching skills for a given goal and returns a dense digest.
-   `magicskills_get(name, [section], [version])`: Fetches high-relevance expert knowledge for a specific skill.

### Operational Support
-   `magicskills_bootstrap(name)`: Generates a structured task checklist directly from a skill's defined workflow.
-   `magicskills_validate_deps(name)`: Checks the host environment for required binary dependencies.
-   `magicskills_add_root(path)`: Dynamically adds and indexes a new skill directory to the server's knowledge base.

### System Support
-   `get_internal_logs(max_lines)`: Accesses the server's internal logs for auditing and debugging.

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
    "magicskills": {
      "command": "/usr/local/bin/mcp-server-magicskills",
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
    "magicskills": {
      "command": "/path/to/dist/mcp-server-magicskills",
      "args": []
    }
  }
}
```

## Use Cases

-   **Standardized Deployments**: Use `magicskills_bootstrap` to ensure every deployment follows the EXACT company-approved steps.
-   **Intent-Based Assistance**: When a user says "I need to fix the database," the system calls `magicskills_match` to suggest the right troubleshooting skill.
-   **Knowledge Transfer**: Document a complex migration as a skill, and any future agent can follow it exactly.

---

Created in Go for flexibility and speed.
