# [Modernization & Stability] Design Doc

- **Date**: 2026-03-23
- **Topic**: Modernization of `prepare-commit-msg`
- **Status**: PROPOSED

## Goals
1. **Consolidated Git Service**: Reduce `git` binary calls from 3 to 2.
2. **Atomic Writing**: Ensure `COMMIT_MSG` is updated atomically to prevent corruption.
3. **Hardened Error Handling**: Resolve all suppressed error handlers.
4. **Logic Isolation**: Extract LLM cleaning logic from orchestration.

---

### Section 1: Consolidated Git Service (Performance)
- **Current**: 3 separate calls to `git` (`--name-status`, `--shortstat`, `--unified=3`).
- **Proposal**: 2-pass implementation:
    1. `git diff --cached --numstat`: Provides filenames, additions, and deletions in one machine-readable stream.
    2. `git diff --cached --unified=3`: Provides the unified diff body for the prompt.
- **Benefits**: Reduced process orchestration overhead.

### Section 2: LLM Cleaner Extraction (Maintainability)
- **Current**: Brittle prefix-based cleaning in `main.go`.
- **Proposal**: Move logic to `internal/llm/cleaner.go`.
- **Improvements**: Use regexes to strip markdown blocks and known filler text across providers.

### Section 3: Stability & Reliability
- **Safety**: Use `os.WriteFile` to a temp file and `os.Rename` for atomic commit message updates.
- **Error Checking**: Systematically handle errors from `os.UserHomeDir` and `os.ReadFile`.
