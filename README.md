# 🪄 MagicSkills Server (v2.1.0) 🪄

A high-performance, modular Model Context Protocol (MCP) server built in Go for
orchestrating and optimizing `.agent/skills` workflows.

## ❓ What is MagicSkills?

MagicSkills is a specialized orchestration layer between your AI coding agent
(like Antigravity or VS Code Copilot) and your skill libraries. It treats your
`.agent/skills` directory as a high-speed, indexed database of expert directions,
allowing the agent to discover, match, and surgically retrieve exactly the
instructions it needs for any task.

## 🚀 Why use this?

- **Token Efficiency**: Instead of loading 5,000-token instruction files, the
  server allows agents to fetch 300-token summaries or specific sections
  (e.g., `## Workflow`).
- **Workspace Intelligence**: Automatically discovers local project-specific
  skills and prioritizes them over global defaults.
- **Performance**: Zero-latency in-memory caching with `fsnotify`
  file-watching for instant indexing.
- **Weighted Discovery**: Intents are matched against Skill Name, Description,
  and Tags with a deterministic scoring system.

---

## 🏗️ Structure of a Skill File (`SKILL.md`)

MagicSkills expects skills to be written in Markdown with **YAML frontmatter**
and **Sectioned Headers**.

### ✨ The "Magic Directive" Section

The `magicskills_summarize` tool is optimized for high-density guidance. It
follows a specific priority when generating a summary:

1. **`## Magic Directive`**: The absolute distilled essence of the skill
   (Max 300 chars).
2. **`## Directive`**: Secondary guidance section.
3. **`## Summary`**: General overview.
4. **Fallback**: First 300 characters of the full content.

Integrating a `## Magic Directive` section into your `SKILL.md` files allows
agents to understand complex instructions instantly without loading the full
file.

### 📄 Skill Template

```markdown
---
name: go-refactoring
description: Expert Go performance and idiom refinement
tags: ["go", "refactoring", "gc-optimizations"]
version: 2.1.0
---

## Magic Directive
Always prioritize Go 1.26 iterators and slices.SortFunc over legacy sort.Slice.

## Workflow
- [ ] Analyze target module
- [ ] Run benchmark
- [ ] Implement iterative optimizations
- [ ] Verify test parity

## Best Practices
- Use strings.Builder.Grow() for strings.
- Avoid large heap pointers in hot loops.
```

---

## 🛠️ Installation & Setup

### 1. Build & Install

```bash
# Clone and navigate to scripts/go/mcp-server-magicskills
make install
```

This builds and installs the binary to `~/.local/bin/mcp-server-magicskills`.

### 2. Configure Environment Variables

MagicSkills uses a hierarchical discovery process:

- **`MAGIC_SKILLS_PATH`**: (Optional) Set this to your global skill library root.
- **Local Discovery**: The server automatically searches upward from CWD to
  find the `.agent/skills` folder in your project.

### 3. Add to `mcp_config.json`

```json
{
  "mcpServers": {
    "magicskills": {
      "command": "/home/adm_saxsmith/.local/bin/mcp-server-magicskills",
      "args": [],
      "env": {
        "MAGIC_SKILLS_PATH": "absolute/path/to/your/skills"
      }
    }
  }
}
```

---

## 🔍 Tool Reference

### **Discovery & Intent**

- `magicskills_list`: Lists all indexed global and local skills with metadata.
- `magicskills_match(intent)`: Suggests the best skill for your current task
  using weighted keyword scoring.

### **Retrieval & Automation**

- `magicskills_summarize(name)`: Returns a high-level "Magic Directive" or
  300-char summary (saves context window).
- `magicskills_get(name)`: Retrieves the full skill contents.
- `magicskills_get_section(name, section)`: surgically retrieves a specific
  section (e.g., "Best Practices").
- `magicskills_bootstrap(name)`: Extracts the "Workflow" section and formats
  it as a checklist for your `task.md`.

### **Observability & Status**

- `magicskills_get_logs`: Fetches the last 512KB of internal server logs
  for debugging.
- `magicskills://status`: (Resource) A real-time dashboard showing indexing health.

---

## 🛑 Production Constraints

- **Concurrency**: Thread-safe with `RWMutex` for all index access.
- **Memory**: Circular log buffer ensures the server never exceeds 1MB
  of memory usage for logs.
- **Safety**: Built-in OS signal handling for graceful shutdown.

---

Created in Go for performance and efficiency.
Built for Antigravity and VS Code context optimization.
