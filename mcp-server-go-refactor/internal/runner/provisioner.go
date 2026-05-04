// Package runner provides functionality for the runner subsystem.
package runner

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnsureToolchain checks if the specified Go toolchain version exists in the isolated cache.
// If not, it downloads, extracts, and returns the absolute path to the 'go' executable.
func EnsureToolchain(ctx context.Context, version string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user cache dir: %w", err)
	}

	toolchainDir := filepath.Join(cacheDir, "mcp-server-go-refactor", "toolchains", "go")
	binName := "go"
	if runtime.GOOS == "windows" {
		binName = "go.exe"
	}

	// For standard go extractions, the binary usually ends up at toolchainDir/go/bin/go
	// However, we will extract contents such that `toolchainDir` itself acts as GOROOT.
	// So we expect: ~/.cache/mcp-server-go-refactor/toolchains/go/bin/go
	expectedBinary := filepath.Join(toolchainDir, "bin", binName)

	if _, err := os.Stat(expectedBinary); err == nil {
		slog.Debug("toolchain already provisioned", "path", expectedBinary)
		return expectedBinary, nil
	}

	slog.Info("provisioning standalone go toolchain...", "version", version, "os", runtime.GOOS, "arch", runtime.GOARCH)

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	url := fmt.Sprintf("https://go.dev/dl/go%s.%s-%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status downloading %s: %s", url, resp.Status)
	}

	// Create temporary directory for extraction to ensure atomicity
	tmpDir, err := os.MkdirTemp(filepath.Dir(toolchainDir), "go-provisioner-tmp-*")
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(toolchainDir), 0755); err != nil {
				return "", fmt.Errorf("failed to create cache dir: %w", err)
			}
			tmpDir, err = os.MkdirTemp(filepath.Dir(toolchainDir), "go-provisioner-tmp-*")
			if err != nil {
				return "", fmt.Errorf("failed to create temp dir: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to create temp dir: %w", err)
		}
	}
	defer os.RemoveAll(tmpDir)

	if ext == "tar.gz" {
		if err := extractTarGz(resp.Body, tmpDir); err != nil {
			return "", fmt.Errorf("failed to extract tar.gz: %w", err)
		}
	} else {
		// zip requires seeking, so we must download to a temp file first
		tmpFile, err := os.CreateTemp("", "go-dl-*.zip")
		if err != nil {
			return "", fmt.Errorf("failed to create temp zip: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			tmpFile.Close()
			return "", fmt.Errorf("failed to download zip: %w", err)
		}
		tmpFile.Close()
		if err := extractZip(tmpFile.Name(), tmpDir); err != nil {
			return "", fmt.Errorf("failed to extract zip: %w", err)
		}
	}

	// The archive usually contains a top-level "go" directory.
	extractedGoDir := filepath.Join(tmpDir, "go")
	if _, err := os.Stat(extractedGoDir); os.IsNotExist(err) {
		return "", fmt.Errorf("expected top-level 'go' directory in archive not found")
	}

	os.RemoveAll(toolchainDir) // clean up if partially exists
	if err := os.Rename(extractedGoDir, toolchainDir); err != nil {
		return "", fmt.Errorf("failed to move extracted toolchain into place: %w", err)
	}

	slog.Info("go toolchain provisioned successfully", "path", expectedBinary)
	return expectedBinary, nil
}

func extractTarGz(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Prevent path traversal
		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(zipFile, dest string) error {
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		destFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(destFile, rc); err != nil {
			destFile.Close()
			rc.Close()
			return err
		}
		destFile.Close()
		rc.Close()
	}
	return nil
}
