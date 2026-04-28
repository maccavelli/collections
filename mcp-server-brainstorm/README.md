# 🧠 Brainstorm MCP Server

A powerful Model Context Protocol (MCP) server designed to facilitate deep technical brainstorming, project discovery,
and architectural decision-making through a Socratic, adversarial approach.

## 🚀 Overview

The Brainstorm server helps engineers and architects stress-test designs, identify project gaps, and document decisions.
It bridges the gap between raw LLM analysis and complex project context by providing structured rubrics, automated
discovery, and a proactive clarification engine.

### 📋 Core Pillars

1. **Requirement Grounding**: Proactively identifies architectural "decision forks" (e.g., Database choice, Auth
   strategy) and uses Socratic questioning to force precise definitions.
2. **Discovery**: Unified scanning of project structure, technology stacks, and documentation to identify gaps or
   technical debt.
3. **Design Analysis**: Interactive stress-testing using adversarial personas (Red Teaming) and multi-dimensional
   quality rubrics (Scalability, Security, Modularity).
4. **Decision Tracking**: Capturing architectural logic and rejected alternatives into structured Architecture Decision
   Records (ADR).

---

## 🛠️ Tools

### `clarify_requirements`

Analyzes high-level requirements to detect architectural "decision forks".

- **Parameter**: `requirements` (string)
- **Usage**: Use this at the start of any project to resolve ambiguity early.

### `discover_project`

Performs a unified scan of the project structure and technology stack.

- **Parameter**: `path` (string, optional)
- **Usage**: Identify documentation gaps and suggest critical next steps.

### `critique_design`

Provides a consolidated, multi-dimensional assessment of a design snippet.

- **Parameter**: `design` (string)
- **Usage**: Audit for scalability, security, and modularity using Red Team personas.

### `analyze_evolution`

Evaluates the risks, breaking changes, and deprecation paths of a proposed extension.

- **Parameter**: `proposal` (string)

### `capture_decision_logic`

Translates architectural discussions into structured ADRs.

- **Parameters**: `decision` (string), `alternatives` (string)

---

## ⚙️ Installation

### 1. Build the Binary

```bash
go build -o dist/mcp-server-brainstorm main.go
```

### 2. Configure for IDEs

#### **Antigravity (Internal Orchestrator)**

Antigravity automatically manages this server. If you need to add it manually to a custom environment:

```yaml
# antigravity/config.yaml
mcpServers:
  brainstorm:
    command: "/absolute/path/to/dist/mcp-server-brainstorm"
```

#### **VS Code (MCP Extension / Cline)**

Add to your `mcp_config.json`:

```json
{
  "mcpServers": {
    "brainstorm": {
      "command": "/absolute/path/to/dist/mcp-server-brainstorm",
      "args": [],
      "env": {
        "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin"
      }
    }
  }
}
```

#### **Cursor IDE**

1. Open **Settings** -> **Features** -> **MCP**.
2. Click **+ Add New MCP Server**.
3. Name: `Brainstorm`
4. Type: `stdio`
5. Command: `/absolute/path/to/dist/mcp-server-brainstorm`

---

## 📖 Use Cases

- **Initial Scoping**: Run `clarify_requirements` to ensure the technical foundation is solid.
- **Pre-Implementation Design Review**: Run a feature proposal through `critique_design` to find potential flaws.
- **Project Onboarding**: Use `discover_project` when starting on a new repository.
- **Architecture Governance**: Use `capture_decision_logic` to document technical pivots.

---

## 💻 CLI Functionality

The binary supports a `-version` flag to check the current build version.

```bash
./mcp-server-brainstorm -version
```

---

*Built with Go for performance and reliability.*
