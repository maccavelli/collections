# 📝 Prepare-Commit-Msg Hook

A high-performance Git hook written in Go to automatically format, validate, and enrich your commit messages.

## 🚀 Overview

`prepare-commit-msg` is a Git hook that ensures every commit message follows your team's standards. It can automatically prepend Jira ticket numbers from the branch name, enforce character limits, and add developer metadata.

### 📋 Core Pillars

1.  **Branch Name Analysis**: Automatically extracts Jira issue keys (e.g., `PROJ-123`) from branch names.
2.  **Commit Standardization**: Enforces a consistent header format (e.g., `[PROJ-123] description`).
3.  **Interactive Prompts**: (If enabled) Prompts the user for additional metadata if missing.
4.  **Blazing Fast**: Written in Go to ensure zero delay when running `git commit`.

---

## 🛠️ Usage & Functionality

### How it Works
When you run `git commit`, this hook is triggered. It performs the following steps:
1.  **Read Branch**: Detects the current Git branch name.
2.  **Extract Info**: Looks for patterns like `PROJ-123` or `feature/xyz`.
3.  **Format Message**: Updates the `.git/COMMIT_EDITMSG` file with the enriched header.
4.  **Validate**: Ensures the message isn't empty and doesn't exceed length limits.

### Manual Usage
You can also run it manually to check what it would do:
```bash
./prepare-commit-msg --branch feature/PROJ-123
```

---

## ⚙️ Installation

### 1. Build the Binary
```bash
make build
```

### 2. Install the Hook
Copy the binary to your `.git/hooks` directory and rename it.
```bash
cp dist/prepare-commit-msg .git/hooks/prepare-commit-msg
chmod +x .git/hooks/prepare-commit-msg
```

---

## 💡 Use Cases

1.  **Jira Traceability**: Ensure every commit is linked to a ticket without manual typing.
2.  **Linting**: Block commits that have generic messages like "fix" or "update".
3.  **Metadata Injection**: Add the current build environment or user ID to the commit footer.

---

*Built with Go for seamless Git workflows.*
