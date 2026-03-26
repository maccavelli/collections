# Go Prepare Commit Message Hook

An AI-powered Git hook that automatically generates descriptive, Conventional Commit messages based on your staged changes.

## 🎯 What it is for
The `prepare-commit-msg` hook eliminates the mental overhead of drafting commit messages. It ensures that your repository history is consistent, detailed, and adheres to the **Conventional Commits** specification. Optimized for **maximum performance and robustness**, it operates with sub-100ms overhead by utilizing in-process Git analysis.

## ✨ Key Features
- **Multi-Provider Support**: Native integration with **Google Gemini**, **Anthropic Claude**, and **OpenAI GPT**.
- **Conventional Commit Standards**: Automatically categorizes changes as `feat`, `fix`, `docs`, `refactor`, `chore`, etc.
- **In-Process Git Analysis**: Uses `go-git` for high-speed repository scanning—no external `git` process spawning required.
- **Zero-Dependency SDKs**: Uses standard Go `net/http` for LLM communication, ensuring a minimal binary footprint and fast startup.
- **Interactive Setup**: Includes a CLI wizard for quick configuration and provider selection.
- **Self-Documenting Configuration**: Automatically maintains empty placeholders for all supported providers in your config file for easy manual updates.

## ⚙️ How it works
1. **Context Gathers**: Analyzes unified diffs and file metadata for all staged changes.
2. **AI Inference**: Sends the context to your preferred LLM provider with a strict Conventional Commit prompt.
3. **Drafting**: Populates your Git commit message editor with the generated draft.
4. **Human Review**: You retain final control—edit the draft or save as-is to complete the commit.

## 🔧 Installation

### 1. Build the Binary
Clone the repository and build the binary for your platform:
```bash
make build
```
The compiled binary will be located in the `dist` directory.

### 2. Install as a Git Hook
Copy the binary to your project's `.git/hooks` folder and name it `prepare-commit-msg`:
```bash
cp dist/prepare-commit-msg-linux-amd64 /path/to/your/project/.git/hooks/prepare-commit-msg
chmod +x /path/to/your/project/.git/hooks/prepare-commit-msg
```

## 🚀 Setup & Usage

### 1. Configure Providers
Run the interactive setup wizard to configure your API keys and preferred model:
```bash
# Navigate to the hooks directory or run globally if in PATH
./prepare-commit-msg --setup
```
Supported providers:
- **Gemini**: Recommended `gemini-2.5-flash-lite` (Default)
- **Anthropic**: Recommended `claude-3-5-haiku-latest`
- **OpenAI**: Recommended `gpt-4o-mini`

### 2. Regular Workflow
Simply stage your changes and run `git commit` as usual:
```bash
git add .
git commit
```
The hook will trigger, generate the message, and open your configured editor with the draft.

## 💡 Detailed Guidance
- **Configuration Path**: Settings are stored in `~/.config/prepare-commit-msg/config.json`.
- **Security**: The configuration file is created with `0600` permissions (read/write by owner only).
- **Bypassing**: If you need to skip the AI generation for a specific commit, use the `--no-verify` flag.
- **Customization**: The tool respects your existing commit message templates if present.

## ⚖️ License
MIT

---
Built with Go for performance and efficiency.
