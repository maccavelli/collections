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

Built in Go for performance and reliability, the server follows a modular architecture:

- **Go-based MCP Server**: Implements the Model Context Protocol for seamless integration with AI IDEs.
- **Provider-based Engine**: Core logic is encapsulated into specialized providers (`Discovery`, `Design`, `Decision`),
  allowing for easy extension of analysis rules.
- **Session Persistence**: Context is maintained in a local `.brainstorm.json` file, allowing tools to provide
  relevant suggestions based on the project history.
- **Middleware Integration**: Structured logging, performance timing, and robust error handling ensure
  production-ready reliability.

### Why it exists (Rationale)

LLMs often lack the specific project context or the "adversarial instinct" required for deep architectural review.
The Brainstorm server provides:

- **Structured Criticality**: Forcing deeper inquiry into failure modes and edge cases.
- **Context Preservation**: Making project-specific metadata (naming conventions, stack details) available to the AI.
- **Process Standardization**: Driving the engineering process from initial discovery to finalized decision
  documentation.

## Tools

### Discovery Tools

- `analyze_project`: Scans a directory for project metadata and identifies documentation/complexity gaps.
- `suggest_next_step`: Analyzes session state to provide the most critical next action.
- `get_internal_logs`: Retrieves server logs for transparency and debugging.

### Design Tools

- `challenge_assumption`: Generates targeted questions about failure modes based on a design snippet.
- `analyze_evolution`: Evaluates proposed changes for risks and deprecation paths.
- `evaluate_quality_attributes`: Audits a design against rubrics (Scalability, Security, etc.).
- `red_team_review`: Simulates adversarial personas to uncover non-obvious failure modes.

### Decision Tools

- `capture_decision_logic`: Generates structured ADRs capturing context and rejected alternatives.

## Getting Started

### Installation

```bash
go build -o brainstorm-server
```

### Usage as an MCP Server

Add the server to your MCP configuration (e.g., in Claude Desktop):

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

The project is modularized for better maintainability:

- `internal/engine`: Core reasoning and analysis providers.
- `internal/handler`: MCP tool handlers and response formatting.
- `internal/models`: Shared communication structures.
- `internal/state`: Session and project metadata management.

---
Created in Go for performance and efficiency.
