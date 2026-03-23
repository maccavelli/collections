# Brainstorm Server

A powerful MCP server designed to facilitate deep technical brainstorming, project discovery, and architectural
decision-making through a Socratic, adversarial approach.

## Overview

The Brainstorm server helps engineers and architects stress-test designs, identify project gaps, and document
decisions. It bridges the gap between raw LLM analysis and complex project context by providing structured rubrics
and automated discovery.

### What it does (Core Pillars)

1. **Discovery**: Unified scanning of project structure, technology stacks, and documentation to identify gaps or
   technical debt.
2. **Design Analysis**: Interactive stress-testing using adversarial personas (Red Teaming) and multi-dimensional
   quality rubrics (Scalability, Security, Modularity).
3. **Decision Tracking**: Capturing architectural logic and rejected alternatives into structured Architecture
   Decision Records (ADR).

### How it works (Architecture)

Built in Go for performance and reliability, the server follows a modular, provider-centric architecture:

- **Unified Processing Engine**: Core reasoning is partitioned into specialized providers (`Discovery`, `Design`,
  `Decision`), allowing for high-density analysis within a single tool invocation.
- **Socratic & Red-Teaming Integration**: The `critique_design` tool uses a multi-dimensional persona system to
  simultaneously audit for quality attributes, security risks, and architectural blind spots.
- **Context-Aware Analytics**: Tools maintain awareness of previous project states via the internal `.brainstorm.json`
  manifest.
- **MCP Provider**: Implements a standard JSON-RPC 2.0 interface for seamless integration into any AI-driven
  development workflow.

### Why it exists (Rationale)

LLMs often suffer from "default compliance," agreeing with flawed designs to avoid conflict.
The Brainstorm server provides:

- **Forced Contention**: Systematic "Red Team" analysis to find non-obvious failure modes.
- **Socratic Scrutiny**: Questions the underlying assumptions of a proposal rather than just suggesting syntax.
- **Institutional Memory**: Documentation of decision logic that persists beyond a single terminal session.

## Tools

### Discovery & Analytics

- `discover_project(path)`: Performs a unified scan of the project structure and technology stack to identify
  documentation gaps and suggest critical next steps.
- `get_internal_logs(max_lines)`: Retrieves recent internal server logs for debugging and transparency.

### Architectural Critique

- `critique_design(design)`: Provides a consolidated, multi-dimensional assessment of a design snippet, using
  Socratic inquiry and Red Team personas to audit for scalability, security, and modularity.
- `analyze_evolution(proposal)`: Evaluates the risks, breaking changes, and deprecation paths of a proposed project
  extension.

### Decision Capture

- `capture_decision_logic(decision, alternatives)`: Translates architectural discussions into structured,
  high-quality Architecture Decision Records (ADR).

## Getting Started

### Installation

```bash
make build
```

The resulting binary will be located in the `dist` directory.

### Usage as an MCP Server

Add the server to your MCP configuration (e.g., in Claude Desktop or Antigravity):

```json
{
  "mcpServers": {
    "brainstorm": {
      "command": "/path/to/mcp-server-brainstorm",
      "args": []
    }
  }
}
```

## Development

The project is structured for high modularity:

- `internal/engine`: Core reasoning engine and persona providers.
- `internal/handler`: MCP request handlers and response mapping.
- `internal/state`: State management for project manifests.
- `internal/models`: Common data structures.

---

Created in Go for performance and efficiency.
