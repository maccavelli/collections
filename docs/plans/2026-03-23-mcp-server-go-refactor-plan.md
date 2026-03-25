# mcp-server-go-refactor Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Create a robust Go MCP server offering 7 advanced AST and local testing tools (`go_dependency_impact`, `go_interface_analyzer`, `go_test_coverage_tracer`, `go_package_cycler`, `go_call_graph_analyzer`, `go_struct_alignment_optimizer`, `go_tag_manager`).

**Architecture:** Component-Based Domain Services with a centralized MCP handler, natively delegating to OS executions (`go test`, `go list`) and `golang.org/x/tools/go/packages`.

**Tech Stack:** Go 1.26.1+, `github.com/mark3labs/mcp-go`, `golang.org/x/tools/go/packages`, `go/ast`.

---

### Task 1: Module Initialization & Scaffolding
**Files:**
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/go.mod`
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/main.go`
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/main_test.go`

**Step 1: Write the failing test**
```go
package main

import "testing"

func TestMainScaffold(t *testing.T) {
    if Version == "" {
        t.Fatal("Version should be set")
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v ./...`
Expected: FAIL "Version not declared by package main"

**Step 3: Write minimal implementation**
```go
package main

var Version = "dev"

func main() {}
```

**Step 4: Run test to verify it passes**
Run: `go test -v ./...`
Expected: PASS

**Step 5: Commit**
```bash
git add main.go main_test.go go.mod
GIT_EDITOR=true git commit
```

---

### Task 2: Implement MCP Handler
**Files:**
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/internal/handler/handler.go`
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/internal/handler/handler_test.go`

**Step 1: Write the failing test**
```go
package handler

import "testing"

func TestHandlerRegistration(t *testing.T) {
    h := NewHandler()
    if h == nil {
        t.Fatal("Handler should not be nil")
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v ./internal/handler`
Expected: FAIL undefined NewHandler

**Step 3: Write minimal implementation**
```go
package handler

type Handler struct {}

func NewHandler() *Handler {
    return &Handler{}
}
```

**Step 4: Run test to verify it passes**
Run: `go test -v ./internal/handler`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/handler/handler.go internal/handler/handler_test.go
GIT_EDITOR=true git commit
```

---

### Task 3: Dependency Impact Analyzer
**Files:**
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/internal/dependency/analyzer.go`
- Create: `/home/adm_saxsmith/gitrepos/saxsmith-global-context/scripts/go/mcp-server-go-refactor/internal/dependency/analyzer_test.go`

**Step 1: Write the failing test**
```go
package dependency

import "testing"

func TestAnalyzeImpact(t *testing.T) {
    impact, err := Analyze("github.com/gin-gonic/gin")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if impact == nil {
        t.Fatal("expected impact data")
    }
}
```

**Step 2: Run test to verify it fails**
Run: `go test -v ./internal/dependency`
Expected: FAIL undefined Analyze

**Step 3: Write minimal implementation**
```go
package dependency

type Impact struct {}

func Analyze(pkg string) (*Impact, error) {
    return &Impact{}, nil
}
```

**Step 4: Run test to verify it passes**
Run: `go test -v ./internal/dependency`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/dependency/analyzer.go internal/dependency/analyzer_test.go
GIT_EDITOR=true git commit
```

---

*(Additional Tasks (4-8) follow the same execution pattern for `astutil`, `coverage`, `graph`, `layout`, and `tags` under the execute-plan flow)*
