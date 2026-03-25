# Brainstorm Server

A powerful MCP server designed to facilitate deep technical brainstorming, project discovery, and architectural decision-making through a Socratic, adversarial approach.

## Overview

The Brainstorm server helps engineers and architects stress-test designs, identify project gaps, and document decisions. It bridges the gap between raw LLM analysis and complex project context by providing structured rubrics, automated discovery, and a proactive clarification engine.

### What it does (Core Pillars)

1.  **Requirement Grounding**: Proactively identifies architectural "decision forks" (e.g., Database choice, Auth strategy) and uses Socratic questioning to force precise definitions before implementation.
2.  **Discovery**: Unified scanning of project structure, technology stacks, and documentation to identify gaps or technical debt.
3.  **Design Analysis**: Interactive stress-testing using adversarial personas (Red Teaming) and multi-dimensional quality rubrics (Scalability, Security, Modularity).
4.  **Decision Tracking**: Capturing architectural logic and rejected alternatives into structured Architecture Decision Records (ADR).

### How it works (Architecture)

Built in Go for performance and reliability, the server follows a modular, provider-centric architecture:

-   **Clarification Engine**: Triggers Socratic prompts when ambiguous components (Database, Auth, API, Queue) are mentioned without sufficient constraints.
-   **Unified Processing Engine**: Core reasoning is partitioned into specialized providers (`Discovery`, `Design`, `Decision`), allowing for high-density analysis within a single tool invocation.
-   **Socratic & Red-Teaming Integration**: The `critique_design` tool uses a multi-dimensional persona system to simultaneously audit for quality attributes, security risks, and architectural blind spots.
-   **Context-Aware Analytics**: Tools maintain awareness of previous project states via the internal `.brainstorm.json` manifest.
-   **MCP Provider**: Implements a standard JSON-RPC 2.0 interface for seamless integration into any AI-driven development workflow.

### Why it exists (Rationale)

LLMs often suffer from "default compliance," agreeing with flawed designs to avoid conflict. The Brainstorm server provides:

-   **Early Ambiguity Detection**: Flags "Decision Forks" early to prevent expensive downstream re-architecture.
-   **Forced Contention**: Systematic "Red Team" analysis to find non-obvious failure modes.
-   **Socratic Scrutiny**: Questions the underlying assumptions of a proposal rather than just suggesting syntax.
-   **Institutional Memory**: Documentation of decision logic that persists beyond a single terminal session.

## Tools

### Requirement Clarification
-   `clarify_requirements(requirements)`: Analyzes high-level requirements to detect architectural "decision forks" (e.g., SQL vs NoSQL, JWT vs Session). It generates targeted Socratic questions to resolve ambiguity early.

### Discovery & Analytics
-   `discover_project([path])`: Performs a unified scan of the project structure and technology stack to identify documentation gaps and suggest critical next steps.
-   `get_internal_logs(max_lines)`: Retrieves the most recent internal server logs for transparency and debugging.

### Architectural Critique
-   `critique_design(design)`: Provides a consolidated, multi-dimensional assessment of a design snippet, using Socratic inquiry and Red Team personas to audit for scalability, security, and modularity.
-   `analyze_evolution(proposal)`: Evaluates the risks, breaking changes, and deprecation paths of a proposed project extension.

### Decision Capture
-   `capture_decision_logic(decision, alternatives)`: Translates architectural discussions into structured, high-quality Architecture Decision Records (ADR).

## Installation

### 1. Build the Binary

Ensure you have Go installed, then build the binary:

```bash
go build -o dist/mcp-server-brainstorm main.go
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
    "brainstorm": {
      "command": "C:\\path\\to\\mcp-server-brainstorm.exe",
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
    "brainstorm": {
      "command": "/usr/local/bin/mcp-server-brainstorm",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

## Use Cases

- **Initial Scoping**: Use `clarify_requirements` at the very beginning of a task to ensure the technical foundation (DB, Auth, API) is correctly chosen.
- **Pre-Implementation Design Review**: Run a feature proposal through `critique_design` to find potential flaws BEFORE writing code.
- **Project Onboarding**: Use `discover_project` when starting on a new repository to find where documentation is missing.
- **Architecture Governance**: Use `capture_decision_logic` to ensure all major technical pivots are documented as ADRs.

---

Created in Go for performance and efficiency.
