package harvest

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ResolveSource securely resolves a URL, local path, remote Go package, or standard library into a local absolute path.
func ResolveSource(ctx context.Context, input string) (string, error) {
	// 1. Is it a URL or explicit protocol? Normalize it.
	parsedPath := input
	if strings.HasPrefix(input, "web://") {
		return "https://" + strings.TrimPrefix(input, "web://"), nil
	} else if strings.HasPrefix(input, "go://") {
		parsedPath = strings.TrimPrefix(input, "go://")
	} else if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		parsedPath = strings.TrimPrefix(input, "https://pkg.go.dev/")
		parsedPath = strings.TrimPrefix(parsedPath, "http://pkg.go.dev/")
		parsedPath = strings.TrimPrefix(parsedPath, "https://")
		parsedPath = strings.TrimPrefix(parsedPath, "http://")
		slog.Info("Input recognized as URL; stripping to go package path", "path", parsedPath)
	}

	// 2. Is it already a local directory?
	if info, err := os.Stat(parsedPath); err == nil && info.IsDir() {
		abs, err := filepath.Abs(parsedPath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		slog.Info("Input recognized as local directory", "path", abs)
		return abs, nil
	}

	// 3. Let's ask Go natively to resolve the path (handles stdlib, already cached mod, etc.)
	slog.Info("Attempting native Go resolution", "pkg", parsedPath)
	cmdListDirect := exec.CommandContext(ctx, "go", "list", "-f", "{{.Dir}}", parsedPath)
	var directOut bytes.Buffer
	cmdListDirect.Stdout = &directOut
	if err := cmdListDirect.Run(); err == nil {
		absPath := strings.TrimSpace(directOut.String())
		if absPath != "" {
			slog.Info("Successfully resolved standard package to local path", "absDir", absPath)
			return absPath, nil
		}
	}

	// 4. Remote Module Needs Downloading
	slog.Info("Package not discovered locally. Initiating native fetch...", "pkg", parsedPath)
	tmpDir, err := os.MkdirTemp("", "resolver-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Decouple remote module fetch from the strict MCP request context to allow long downloads
	// This prevents large modules (like x/tools) from being SIGKILL'd by the proxy timeout
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Create an isolated workspace to download the package without polluting user's project
	cmdInit := exec.CommandContext(fetchCtx, "go", "mod", "init", "tmp")
	cmdInit.Dir = tmpDir
	if err := cmdInit.Run(); err != nil {
		slog.Warn("go mod init failed in temp resolver dir", "error", err)
	}

	// Fetch the remote module via standard go get to ensure transient dependencies are pulled
	// Dynamically extract trailing semantic versions to form a structurally valid retrieval syntax
	basePkg := parsedPath
	targetVersion := "/...@latest"
	if idx := strings.LastIndex(parsedPath, "@"); idx != -1 {
		basePkg = parsedPath[:idx]
		targetVersion = "/..." + parsedPath[idx:]
	}

	cmdGet := exec.CommandContext(fetchCtx, "go", "get", basePkg+targetVersion)
	cmdGet.Dir = tmpDir
	var getStderr bytes.Buffer
	cmdGet.Stderr = &getStderr
	if err := cmdGet.Run(); err != nil {
		slog.Warn("Failed to fetch Go module, falling back to heuristic Web Scraper", "pkg", parsedPath, "getStderr", getStderr.String())
		if strings.Contains(basePkg, ".") {
			return "https://" + parsedPath, nil // Cascade dynamically
		}
		return "", fmt.Errorf("Could not fetch remote source '%s'. Ensure package exists and is accessible. err: %w, stderr: %s", parsedPath, err, getStderr.String())
	}

	// Request the absolute directory dynamically of the package
	cmdListDown := exec.CommandContext(fetchCtx, "go", "list", "-f", "{{.Dir}}", basePkg)
	cmdListDown.Dir = tmpDir
	var downOut bytes.Buffer
	var listStderr bytes.Buffer
	cmdListDown.Stdout = &downOut
	cmdListDown.Stderr = &listStderr
	if err := cmdListDown.Run(); err == nil {
		absPath := strings.TrimSpace(downOut.String())
		if absPath != "" {
			slog.Info("Successfully fetched and resolved remote package into mod cache", "absDir", absPath)
			return absPath, nil
		}
	} else {
		slog.Warn("Failed to resolve absolute mod cache path via go list, triggering cascade", "err", err.Error(), "stderr", listStderr.String())
		if strings.Contains(basePkg, ".") {
			return "https://" + parsedPath, nil // Cascade dynamically
		}
	}

	// Fallback to returning the raw input if all else fails
	return parsedPath, nil
}
