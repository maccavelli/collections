# Go Prepare Commit Message Hook

An AI-powered Git hook that automatically generates descriptive, Conventional Commit messages.

## 🎯 What it is for

The `prepare-commit-msg` hook eliminates the mental overhead of drafting commit messages. It ensures that your
repository history is consistent, detailed, and adheres to the Conventional Commits specification without
requiring manual effort.

## ⚙️ How it works

1. **Diff Analysis**: Scans staged changes to gather file statuses and full unified diffs.
2. **AI Orchestration**: Constructs a high-context prompt for Gemini or OpenAI.
3. **Hook Integration**: Leverages the standard Git `prepare-commit-msg` hook flow.

## 🧠 Why it works

- **Context-Awareness**: It understands the *logic* of your code changes.
- **Standards Enforcement**: Defaults to Conventional Commits (feat, fix, docs, etc.).
- **Native Performance**: Written in Go for zero-latency execution.

## 🚀 Usage Instructions

1. **Configure**: Run the setup wizard to choose your provider and model.

   ```bash
   ./prepare-commit-msg --setup
   ```

2. **Commit**: Work as usual. The tool automatically drafts the message.
3. **Edit/Save**: Review the AI-generated draft in your editor and save.

## 🔧 Installation Instructions

1. **Copy Binary**: Copy the built binary into your project's `.git/hooks/` directory.

   ```bash
   cp prepare-commit-msg-linux-amd64 .git/hooks/prepare-commit-msg
   ```

2. **Make Executable**:

   ```bash
   chmod +x .git/hooks/prepare-commit-msg
   ```

## 💡 Detailed Guidance

- **Model Selection**: For the best balance of speed and quality, the tool recommends `gemini-2.5-flash-lite`.
- **Security**: API keys are stored in `~/.config/prepare-commit-msg/config.json`.
- **Customization**: You can override the generated message entirely by simply deleting it in your editor before saving.

---
*Built with Go for performance and stability.*
