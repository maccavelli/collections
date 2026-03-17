# Go Prepare Commit Message Hook

`prepare-commit-msg` is a standalone Git hook that leverages artificial intelligence to automatically
generate descriptive, well-structured, and compliant Conventional Commit messages based on your staged changes.

## Why AI-Generated Commit Messages?

Standard commit messages are often rushed, vague, or inconsistent. By leveraging AI, this tool significantly
improves your repository's history:

- **Consistency:** Automatically adheres to the [Conventional Commits](https://www.conventionalcommits.org/)
  specification.
- **Context-Aware:** Analyzes the actual code diff to understand *what* changed and *why*.
- **Quality:** Generates detailed body descriptions that help teammates understand technical decisions without
  digging through the diff themselves.
- **Efficiency:** Saves developers time by drafting the boilerplate and structure, leaving only the final review
  to the human.

## How it Works

1. **Deep Analysis:** The tool scans your staged changes, gathering file statuses, addition/deletion counts, and the
   full unified diff.
2. **Intelligent Prompting:** It constructs a high-context prompt that summarizes the technical changes for the
   selected LLM (Gemini or OpenAI).
3. **Native Generation:** Using built-in SDKs, it communicates directly with the AI provider to generate a draft
   message.
4. **Draft Review:** The tool prepends the AI's suggestion to your `COMMIT_EDITMSG` file, allowing you to edit or
   approve it before finalizing the commit.

## Installation and Usage

### 1. Configuration (`--setup`)

Before using the hook, you must configure your AI provider and API key.

```bash
./prepare-commit-msg-linux-amd64 --setup
```

**Features of Setup:**

- **Interactive Provider Selection:** Choose between Gemini (default) or OpenAI.
- **Smart Key Detection:** If `GEMINI_API_KEY` or `OPENAI_API_KEY` is set in your environment, the tool will
  automatically import it.
- **Model Selection:** Choose from recommended models like `gemini-2.5-flash-lite`.

Settings are stored securely in `~/.config/prepare-commit-msg/config.json`.

### 2. Install as a Git Hook

To integrate the tool into your repository:

1. **Navigate to your repository's hooks directory:**

   ```bash
   cd /path/to/your/repo/.git/hooks
   ```

2. **Create the `prepare-commit-msg` file:**
   Simply copy the binary into your `.git/hooks/` directory, renaming it to `prepare-commit-msg`.

   ```bash
   cp /path/to/prepare-commit-msg-linux-amd64 prepare-commit-msg
   ```

3. **Make it executable:**

   ```bash
   chmod +x .git/hooks/prepare-commit-msg
   ```

### 3. Commit Your Changes

Work as usual! When you run `git commit`, the tool will trigger:

   ```bash
   git add .
   git commit
   # [Tool] Generating commit message via gemini (gemini-2.5-flash-lite)...
   ```

The AI-drafted message will appear in your editor. Review, save, and you're done!

---

*Built with Go for performance and stability.*
