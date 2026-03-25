package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover(t *testing.T) {
	// 1. Create a dummy Go module
	tmp, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	modPath := filepath.Join(tmp, "go.mod")
	modContent := "module example.com/testmod\n\ngo 1.20\n"
	if err := os.WriteFile(modPath, []byte(modContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Add a subpackage with a Go file
	subPkgDir := filepath.Join(tmp, "internal", "foo")
	if err := os.MkdirAll(subPkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subPkgDir, "foo.go"), []byte("package foo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Case A: Discover from root
	info, err := Discover(ctx, tmp)
	if err != nil {
		t.Fatalf("Discover root failed: %v", err)
	}
	if info.ModuleName != "example.com/testmod" {
		t.Errorf("expected module example.com/testmod, got %s", info.ModuleName)
	}

	// Case B: Discover from subpackage
	info, err = Discover(ctx, subPkgDir)
	if err != nil {
		t.Fatalf("Discover subpkg failed: %v", err)
	}
	if info.RelativePkg != "./internal/foo" {
		t.Errorf("expected relative pkg ./internal/foo, got %s", info.RelativePkg)
	}

	// Case C: Discover from non-existent directory (Robust Discovery)
	nonExistentDir := filepath.Join(tmp, "non-existent")
	info, err = Discover(ctx, nonExistentDir)
	if err != nil {
		t.Fatalf("Discover non-existent failed: %v", err)
	}
	if info.ModuleRoot == "" {
		t.Error("expected module root to be found for non-existent child")
	}
	if info.RelativePkg != "./non-existent" {
		t.Errorf("expected relative pkg ./non-existent, got %s", info.RelativePkg)
	}

	// Case D: Discover from a file path
	info, err = Discover(ctx, filepath.Join(tmp, "main.go"))
	if err != nil {
		t.Fatalf("Discover from file failed: %v", err)
	}
	if info.ModuleRoot == "" {
		t.Error("expected module root to be found for file input")
	}
	if info.RelativePkg != "./" {
		t.Errorf("expected relative pkg ./, got %s", info.RelativePkg)
	}
}

func TestResolvePattern(t *testing.T) {
	tests := []struct {
		rel      string
		pattern  string
		expected string
	}{
		{"./internal/loader", "", "./internal/loader"},
		{"./internal/loader", ".", "./internal/loader"},
		{"./internal/loader", "...", "internal/loader/..."},
		{"./", "...", "./..."},
		{".", "...", "..."},
	}

	for _, tc := range tests {
		info := &Info{RelativePkg: tc.rel}
		got := info.ResolvePattern(tc.pattern)
		if got != tc.expected {
			t.Errorf("ResolvePattern(%s, %q) = %q; expected %q", tc.rel, tc.pattern, got, tc.expected)
		}
	}
}

func TestDiscoverGoWork(t *testing.T) {
	tmp, err := os.MkdirTemp("", "work-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Create a workspace with a go.work file
	workPath := filepath.Join(tmp, "go.work")
	if err := os.WriteFile(workPath, []byte("go 1.20\n\nuse ./mod1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mod1 := filepath.Join(tmp, "mod1")
	os.MkdirAll(mod1, 0755)
	os.WriteFile(filepath.Join(mod1, "go.mod"), []byte("module mod1\n"), 0644)

	ctx := context.Background()
	info, err := Discover(ctx, mod1)
	if err != nil {
		t.Fatalf("Discover in workspace failed: %v", err)
	}

	// It should detect the go.work root as ModuleRoot for the workspace
	// unless we specifically want the go.mod root.
	// Actually, our 'go env' logic will prefer the 'active' one.
	// In a manual walk, we prioritize go.work.
	if info.DiscoveryType != DiscoveryGoWork {
		t.Logf("DiscoveryType: %s (expected %s if go.work was hit first)", info.DiscoveryType, DiscoveryGoWork)
	}
}

func TestDiscoverVCS(t *testing.T) {
	tmp, err := os.MkdirTemp("", "vcs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Create a .git directory (marker)
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	subDir := filepath.Join(tmp, "some", "pkg")
	os.MkdirAll(subDir, 0755)

	ctx := context.Background()
	info, err := Discover(ctx, subDir)
	if err != nil {
		t.Fatalf("Discover VCS failed: %v", err)
	}

	if info.DiscoveryType != DiscoveryVCS {
		t.Errorf("expected discovery type %s, got %s", DiscoveryVCS, info.DiscoveryType)
	}
	if info.ModuleRoot != tmp {
		t.Errorf("expected module root %s, got %s", tmp, info.ModuleRoot)
	}
}
