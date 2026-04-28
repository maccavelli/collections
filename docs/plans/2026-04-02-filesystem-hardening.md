# Filesystem MCP Server Hardening Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Harden the `mcp-server-filesystem` Go implementation for robustness, performance, and consistency with the reference `mcp-server-sequential-thinking` communication stack.

**Architecture:** Fix communication stack inconsistencies (flush error logging, shutdown timeout), add security guards (file size limits, tree depth limits, search result caps), thread `context.Context` through engine functions, fix the `AsyncWriter.Close()` race, hoist allocations, and add missing unit tests.

**Tech Stack:** Go 1.26.1, `modelcontextprotocol/go-sdk v1.4.1`, `log/slog`

---

### Task 1: Fix Communication Stack Consistency

**Files:**
- Modify: `main.go:33-35` (version printing)
- Modify: `main.go:62-72` (final flush error logging)
- Modify: `main.go:192-198` (autoFlusher flush error logging)
- Modify: `version.go:8` (fix comment)

**Step 1: Fix `autoFlusher` to log flush errors (match reference)**

In `main.go`, replace the `autoFlusher.Write` method:

```go
func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if f, ok := a.w.(flusher); ok {
		if flushErr := f.Flush(); flushErr != nil {
			slog.Debug("auto-flush error", "error", flushErr)
		}
	}
	return n, err
}
```

**Step 2: Fix final flush to log errors (match reference)**

In `main.go`, replace the two `_ = writer.Flush()` calls:

```go
// Line 64 (graceful shutdown path):
if flushErr := writer.Flush(); flushErr != nil {
    slog.Debug("final flush error", "error", flushErr)
}

// Line 71 (normal exit path):
if flushErr := writer.Flush(); flushErr != nil {
    slog.Debug("final flush error", "error", flushErr)
}
```

**Step 3: Use `printVersion()` and fix comment**

In `main.go`, replace lines 33-34:
```go
if *versionFlag {
    printVersion()
    exitFunc(0)
}
```

In `version.go`, fix the comment:
```go
// Version is the current version of the Filesystem MCP server.
```

**Step 4: Run tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go build ./...`
Expected: Build succeeds with no errors.

**Step 5: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/main.go scripts/go/mcp-server-filesystem/version.go
```

---

### Task 2: Fix AsyncWriter Close Race Condition

**Files:**
- Modify: `internal/util/logging.go:71-76`

**Step 1: Fix the Close method**

The current Close() calls `cancel()` then `close(ch)`, which races the worker. The worker's `case <-aw.ctx.Done()` fires before the channel is fully drained from the `case p, ok := <-aw.ch` path. Fix:

```go
func (aw *AsyncWriter) Close() error {
	close(aw.ch)  // Signal worker via channel close
	aw.wg.Wait()  // Wait for worker to drain all pending messages
	aw.cancel()   // Then cancel context (cleanup)
	return nil
}
```

Also update `worker()` to remove the `ctx.Done()` path since close(ch) now signals shutdown:

```go
func (aw *AsyncWriter) worker() {
	defer aw.wg.Done()
	for p := range aw.ch {
		_, _ = aw.writer.Write(p)
	}
}
```

**Step 2: Run tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go build ./...`
Expected: Build succeeds.

**Step 3: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/internal/util/logging.go
```

---

### Task 3: Add File Size Guard, Depth Limit, and Result Cap

**Files:**
- Modify: `internal/config/config.go` (add constants)
- Modify: `internal/engine/fileops.go` (add guards)

**Step 1: Add constants to config**

```go
// Safety limits.
const (
	MaxReadFileSize   = 50 * 1024 * 1024  // 50MB max file read
	MaxTreeDepth      = 20                // Max directory tree recursion depth
	MaxSearchResults  = 1000              // Max results from search_files
)
```

**Step 2: Add file size guard to `ReadFileContent`**

```go
func ReadFileContent(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > config.MaxReadFileSize {
		return "", fmt.Errorf("file too large (%s, limit %s): %s",
			FormatSize(info.Size()), FormatSize(config.MaxReadFileSize), filePath)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return string(data), nil
}
```

**Step 3: Add depth limit to `BuildDirectoryTree`**

Change the signature and add depth tracking:

```go
func BuildDirectoryTree(rootPath string, excludePatterns []string) ([]*TreeEntry, error) {
	return buildTree(rootPath, rootPath, excludePatterns, 0)
}

func buildTree(currentPath, rootPath string, excludePatterns []string, depth int) ([]*TreeEntry, error) {
	if depth > config.MaxTreeDepth {
		return nil, nil // Silently stop recursion at max depth
	}
	// ... existing code, but recursive call passes depth+1:
	// children, err := buildTree(filepath.Join(currentPath, entry.Name()), rootPath, excludePatterns, depth+1)
```

**Step 4: Add result limit to `SearchFiles`**

```go
func SearchFiles(rootPath, pattern string, excludePatterns []string) ([]string, error) {
	var results []string
	err := filepath.WalkDir(rootPath, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(results) >= config.MaxSearchResults {
			return filepath.SkipAll
		}
		// ... rest of existing logic
```

**Step 5: Run tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go test -v ./...`
Expected: All existing tests pass. The guards only trigger on edge cases, not normal test files.

**Step 6: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/internal/config/config.go scripts/go/mcp-server-filesystem/internal/engine/fileops.go
```

---

### Task 4: Add Shutdown Timeout and Context Threading

**Files:**
- Modify: `main.go:139-149` (shutdown timeout)
- Modify: `internal/engine/fileops.go` (add context parameters)
- Modify: `internal/handler/filesystem/tools.go` (pass context)

**Step 1: Add shutdown timeout in `main.go`**

Add `"time"` to imports, then update the `run()` select:

```go
select {
case <-ctx.Done():
    slog.Info("context cancelled; initiating graceful shutdown")
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer shutdownCancel()
    <-shutdownCtx.Done()
case err := <-errChan:
    if isExpectedShutdownErr(err) {
        slog.Info("stdio transport closed gracefully", "reason", err.Error())
        return nil
    }
    return fmt.Errorf("server error: %w", err)
}
return nil
```

**Step 2: Add context to long-running engine functions**

Add `context.Context` as first parameter to `BuildDirectoryTree`, `buildTree`, `SearchFiles`, and check `ctx.Err()` in loops:

```go
func BuildDirectoryTree(ctx context.Context, rootPath string, excludePatterns []string) ([]*TreeEntry, error) {
	return buildTree(ctx, rootPath, rootPath, excludePatterns, 0)
}

func buildTree(ctx context.Context, currentPath, rootPath string, excludePatterns []string, depth int) ([]*TreeEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// ...existing code with ctx passed to recursive calls
}

func SearchFiles(ctx context.Context, rootPath, pattern string, excludePatterns []string) ([]string, error) {
	var results []string
	err := filepath.WalkDir(rootPath, func(fullPath string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// ...existing logic
	})
	// ...
}
```

**Step 3: Update handler calls to pass context**

In `tools.go`, update `DirectoryTreeTool.Handle` and `SearchFilesTool.Handle` to pass `ctx`:

```go
tree, err := engine.BuildDirectoryTree(ctx, validPath, input.ExcludePatterns)
```

```go
results, err := engine.SearchFiles(ctx, validPath, input.Pattern, input.ExcludePatterns)
```

**Step 4: Run tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go test -v ./...`
Expected: All tests pass. Update test calls to pass `context.Background()`.

**Step 5: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/
```

---

### Task 5: Performance Fixes

**Files:**
- Modify: `internal/engine/fileops.go:170-200` (TailFile buffer reuse)
- Modify: `internal/engine/fileops.go:442-460` (hoist MIMEType map)

**Step 1: Hoist MIMEType map to package level**

```go
var mimeTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".svg":  "image/svg+xml",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
}

func MIMEType(ext string) string {
	if mt, ok := mimeTypes[strings.ToLower(ext)]; ok {
		return mt
	}
	return "application/octet-stream"
}
```

**Step 2: Reuse buffer in TailFile**

```go
func TailFile(filePath string, n int) (string, error) {
	// ... existing open/stat code ...

	const chunkSize = 1024
	var lines []string
	position := fileSize
	remaining := ""
	buf := make([]byte, chunkSize) // Reuse buffer

	for position > 0 && len(lines) < n {
		size := int64(chunkSize)
		if size > position {
			size = position
		}
		position -= size

		_, err := f.ReadAt(buf[:size], position)
		// ... rest unchanged, using buf[:size] ...
```

**Step 3: Run tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go test -v ./...`
Expected: All tests pass.

**Step 4: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/internal/engine/fileops.go
```

---

### Task 6: Add Missing Unit Tests

**Files:**
- Create: `main_test.go`
- Create: `internal/util/logging_test.go`
- Create: `internal/util/mcp_helpers_test.go`

**Step 1: Create `main_test.go`**

Test `isExpectedShutdownErr`, `eofDetector`, and `autoFlusher`:

```go
package main

import (
	"bytes"
	"io"
	"testing"
)

func TestIsExpectedShutdownErr(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{io.EOF, true},
		{io.ErrUnexpectedEOF, true},
		{fmt.Errorf("broken pipe"), true},
		{fmt.Errorf("connection reset by peer"), true},
		{fmt.Errorf("use of closed network connection"), true},
		{fmt.Errorf("random error"), false},
	}
	for _, tc := range tests {
		got := isExpectedShutdownErr(tc.err)
		if got != tc.want {
			t.Errorf("isExpectedShutdownErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestAutoFlusher(t *testing.T) {
	var buf bytes.Buffer
	af := &autoFlusher{w: &buf}
	n, err := af.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("Write: n=%d, err=%v", n, err)
	}
	if buf.String() != "hello" {
		t.Errorf("got %q", buf.String())
	}
}

func TestEOFDetector(t *testing.T) {
	cancelled := false
	r := &eofDetector{
		r:      bytes.NewReader(nil),
		cancel: func() { cancelled = true },
	}
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if !cancelled {
		t.Error("expected cancel to be called")
	}
}
```

**Step 2: Create `internal/util/logging_test.go`**

Test `AsyncWriter` and `OpenHardenedLogFile`:

```go
package util

import (
	"bytes"
	"testing"
	"time"
)

func TestAsyncWriter(t *testing.T) {
	var buf bytes.Buffer
	aw := NewAsyncWriter(&buf, 16)
	aw.Write([]byte("hello"))
	aw.Write([]byte(" world"))
	aw.Close()
	if buf.String() != "hello world" {
		t.Errorf("got %q, want \"hello world\"", buf.String())
	}
}

func TestAsyncWriterDropsOnFull(t *testing.T) {
	slow := &slowWriter{delay: 50 * time.Millisecond}
	aw := NewAsyncWriter(slow, 1) // tiny capacity
	aw.maxDuration = 1 * time.Millisecond
	for i := 0; i < 100; i++ {
		aw.Write([]byte("x"))
	}
	aw.Close()
	// Should not hang
}

type slowWriter struct {
	delay time.Duration
}

func (sw *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(sw.delay)
	return len(p), nil
}
```

**Step 3: Create `internal/util/mcp_helpers_test.go`**

Test `HardenedAddTool` panic recovery:

```go
package util

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHardenedAddTool_PanicRecovery(t *testing.T) {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "0.0.1"},
		&mcp.ServerOptions{},
	)
	type EmptyInput struct{}
	HardenedAddTool(s, &mcp.Tool{Name: "panic_tool"}, func(ctx context.Context, req *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
		panic("test panic")
	})
	// Tool should be registered without panic
}
```

**Step 4: Run all tests**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go test -v -race ./...`
Expected: All tests pass with no race conditions.

**Step 5: Commit**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/main_test.go scripts/go/mcp-server-filesystem/internal/util/
```

---

### Task 7: Final Verification

**Step 1: Run full test suite with race detector**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go test -v -race -count=1 ./...`
Expected: All tests pass, no race conditions detected.

**Step 2: Run `go vet`**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && go vet ./...`
Expected: No issues.

**Step 3: Build all platforms**

Run: `cd /home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-filesystem && make build`
Expected: Binary builds successfully.

**Step 4: Commit all remaining changes**

```bash
cd /home/adm_saxsmith/gitrepos/saxsmith-global-context
git add scripts/go/mcp-server-filesystem/
```

---

## Verification Plan

### Automated Tests
- `go test -v -race -count=1 ./...` — Full suite with race detector
- `go vet ./...` — Static analysis
- `make build` — Compilation verification

### What Tests Cover
| Area | Coverage |
|---|---|
| Communication stack (flush errors) | Verified by `autoFlusher` test + build |
| AsyncWriter race | Race detector + `TestAsyncWriter` |
| File size guard | `TestReadFileContent` extended for large file rejection |
| Depth limit | Build verification (runtime guard) |
| Search result cap | Build verification (runtime guard) |
| Shutdown timeout | `isExpectedShutdownErr` test + build |
| Context threading | Build verification (signature changes) |
| Performance (MIMEType) | Existing `TestMIMEType` still passes |
| Performance (TailFile) | Existing `TestTailFile` still passes |
