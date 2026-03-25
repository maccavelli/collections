# Go Prepare Commit Message Hook

An AI-powered Git hook that automatically generates descriptive, Conventional Commit messages.

## 🎯 What it is for
The `prepare-commit-msg` hook eliminates the mental overhead of drafting commit messages. It ensures that your
repository history is consistent, detailed, and adheres to the Conventional Commits specification. Optimized for
**maximum performance and robustness**, it operates with sub-100ms overhead by utilizing in-process Git analysis.

## ⚙️ How it works
1. **In-Process Diff Analysis**: Uses the `go-git` library to perform in-memory repository analysis, eliminating the overhead of spawning external `git` processes.
2. **Concurrent Initialization**: Parallelizes configuration loading and repository scanning to minimize the critical path before AI request initiation.
3. **SDK-less LLM Integration**: Communicates directly with LLM providers using standard Go `net/http` implementations, bypassing heavy third-party SDKs for faster startup and smaller binary size.
4. **IDE Integration**: Seamlessly integrates with VSCode, Antigravity, and standard CLI `git commit` workflows.

## 🧠 Why it works
- **Context-Awareness**: Gathers precise file metadata and unified diffs to understand the *logic* of your changes.
- **Maximum Speed**: Built for zero-latency with concurrent, in-process execution.
- **Robust & Lightweight**: Native Go implementation with zero external runtime dependencies and a minimal binary footprint.
- **Standards Enforcement**: Defaults to Conventional Commits (feat, fix, docs, etc.) with configurable retry and timeout logic.

## 🔧 Installation Instructions

1. **Build**:

   ```bash
   make build
   ```

2. **Copy Binary**: Copy the built binary into your project's `.git/hooks/` directory.

   ```bash
   cp dist/prepare-commit-msg-linux-amd64 .git/hooks/prepare-commit-msg
   ```

3. **Make Executable**:

   ```bash
   chmod +x .git/hooks/prepare-commit-msg
   ```

## 🚀 Usage Instructions

1. **Configure**: Run the setup wizard to choose your provider and model.

   ```bash
   ./prepare-commit-msg --setup
   ```

2. **Commit**: Work as usual. The tool automatically drafts the message.
3. **Edit/Save**: Review the AI-generated draft in your editor and save.

## 💡 Detailed Guidance

- **Model Selection**: For the best balance of speed and quality, the tool recommends `gemini-2.0-flash-lite` or `claude-3-5-haiku-latest`.
- **Security**: API keys are stored in `~/.config/prepare-commit-msg/config.json`.
- **Customization**: You can override the generated message entirely by simply deleting it in your editor before saving.

## ⚖️ License

MIT

___________________________________________________________________________

Created in Go for performance and efficiency.
