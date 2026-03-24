# Modernization Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Refactor `prepare-commit-msg` for better stability, performance (2-pass git), and reliability (atomic write).

**Architecture:** 
- Consolidate 3 git calls into 2 (numstat + diff).
- Move LLM output cleaning logic out of orchestration.
- Implement atomic rename for commit message updates.
- Remove all suppressed `_` errors.

**Tech Stack:** Go 1.26.1+, Standard Library, Git.

---

### Task 1: Stability & Error Handling Fixes
**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/git/git.go:80`
- Modify: `main.go:185`

**Step 1: Replace blank identifiers with proper checks**
- In `internal/config/config.go`, handle `os.UserHomeDir` error in `Load` and `migrateConfig`.
- In `internal/git/git.go:80`, handle `git diff --shortstat` failure (return error or log).
- In `main.go:185`, return error if `os.ReadFile` fails in `writeMessage`.

**Step 2: Commit staged changes**
Run: `git add internal/config/config.go internal/git/git.go main.go`
(User will manually commit)

### Task 2: Performance (Consolidated Git Gathering)
**Files:**
- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Refactor GatherInfo to 2-pass approach**
- Use `git diff --cached --numstat` to parse file names and counts in one pass.
- Use `git diff --cached --unified=3` to get content.
- Update `Info` struct if needed.

**Step 2: Run verification tests**
Run: `go test ./internal/git/... -v`
Expected: PASS

**Step 3: Commit staged changes**
Run: `git add internal/git/*`

### Task 3: Reliability & Maintenance Refactor
**Files:**
- Create: `internal/llm/cleaner.go`
- Modify: `main.go`

**Step 1: Extract cleaner and implement atomic write**
- Define `Clean` function in `internal/llm/cleaner.go` with robust regex-based stripping.
- Update `main.go:writeMessage` to use `os.WriteFile` on a `.tmp` file followed by `os.Rename`.

**Step 2: Final Verification**
Run: `go test ./...`
Expected: PASS

**Step 3: Commit staged changes**
Run: `git add internal/llm/cleaner.go main.go`
