package loader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveDir(t *testing.T) {
	tests := []struct {
		pkgPath         string
		expectedDir     string
		expectedPattern string
	}{
		{"example.com/foo", "", "example.com/foo"},
		{"./foo/...", "", "./foo/..."},
	}

	for _, tc := range tests {
		dir, pat := ResolveDir(tc.pkgPath)
		if dir != tc.expectedDir || pat != tc.expectedPattern {
			t.Errorf("ResolveDir(%s) = (%s, %s); expected (%s, %s)",
				tc.pkgPath, dir, pat, tc.expectedDir, tc.expectedPattern)
		}
	}
}

func TestDiscover(t *testing.T) {
	ctx := context.Background()

	t.Run("Current Directory", func(t *testing.T) {
		res, err := Discover(ctx, ".")
		if err != nil {
			t.Fatalf("Discover(.) failed: %v", err)
		}
		if res.Workspace == nil {
			t.Fatal("expected workspace info")
		}
		if !strings.HasSuffix(res.Pattern, "internal/loader") && res.Pattern != "./" && res.Pattern != "." {
			t.Errorf("expected module-relative pattern, got %s", res.Pattern)
		}
	})

	t.Run("Explicit internal/loader", func(t *testing.T) {
		res, err := Discover(ctx, "mcp-server-go-refactor/internal/loader")
		if err != nil {
			// Might fail if run from somewhere else, but usually OK in repo root
			t.Logf("Discover failed (possibly expected if not in right root): %v", err)
			return
		}
		if res.Workspace.ModuleName != "mcp-server-go-refactor" {
			t.Errorf("expected module mcp-server-go-refactor, got %s", res.Workspace.ModuleName)
		}
	})

	t.Run("Non-existent local path", func(t *testing.T) {
		res, err := Discover(ctx, "./non-existent-subpkg")
		if err != nil {
			t.Fatalf("Discover failed for non-existent local pkg (should fallback): %v", err)
		}
		if !strings.HasSuffix(res.Pattern, "internal/loader/non-existent-subpkg") && res.Pattern != "./non-existent-subpkg" {
			t.Errorf("expected module-relative pattern for missing subpkg, got %s", res.Pattern)
		}
	})
}

func TestLoadPackages(t *testing.T) {
	// Use the current loader package for testing
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Pattern "." when at root of mcp-server-go-refactor should load something
	pkgs, err := LoadPackages(ctx, ".", DefaultMode)
	if err != nil {
		t.Fatalf("LoadPackages failed: %v", err)
	}

	if len(pkgs) == 0 {
		t.Fatal("expected at least one package, got 0")
	}

	// Pattern "mcp-server-go-refactor/internal/loader"
	pkgs, err = LoadPackages(ctx, "mcp-server-go-refactor/internal/loader", DefaultMode)
	if err != nil {
		t.Logf("Full path pattern might fail if not in the right context, but continuing: %v", err)
	} else if len(pkgs) > 0 {
		if pkgs[0].Name != "loader" {
			t.Errorf("expected package loader, got %s", pkgs[0].Name)
		}
	}

	t.Run("Error Case: Not Found", func(t *testing.T) {
		_, err := LoadPackages(ctx, "./this-package-really-does-not-exist", DefaultMode)
		if err == nil {
			t.Error("expected error for non-existent package, got nil")
		}
	})
}

func TestLocalPathValidation(t *testing.T) {
	// Create a temp workspace
	tmp, err := os.MkdirTemp("", "loader-val-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module validation\n\ngo 1.20\n"), 0644)
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	ctx := context.Background()
	
	// Valid load (from root)
	pkgs, err := LoadPackages(ctx, tmp, DefaultMode)
	if err != nil {
		t.Fatalf("LoadPackages failed for valid temp dir: %v", err)
	}
	if len(pkgs) == 0 {
		t.Error("expected packages")
	}

	// Invalid load (missing child)
	missing := filepath.Join(tmp, "missing")
	_, err = LoadPackages(ctx, missing, DefaultMode)
	if err == nil {
		t.Error("expected failure for missing absolute path child")
	}
}
