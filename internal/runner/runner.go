package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner provides a hardened interface for Go command execution.
type Runner struct {
	Dir      string // Execution working directory
	SlogLevel string // slog level if needed
}

// Result captures the result of a Go command.
type Result struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// New creates a new Go runner from a directory.
func New(dir string) *Runner {
	return &Runner{Dir: dir}
}

// RunGo executes a 'go' command from the configured directory.
func (r *Runner) RunGo(ctx context.Context, subCmd string, args ...string) (*Result, error) {
	tctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	// 1. Filter out empty arguments to avoid the 'go: module : not a known dependency' bug
	cleanArgs := []string{subCmd}
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			cleanArgs = append(cleanArgs, arg)
		}
	}

	cmd := exec.CommandContext(tctx, "go", cleanArgs...)
	cmd.Dir = r.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return &Result{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
			Err:    fmt.Errorf("go command failed: %w: %s", err, strings.TrimSpace(stderr.String())),
		}, nil
	}

	return &Result{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
		Err:    err,
	}, nil
}

// Success returns true if the command succeeded.
func (res *Result) Success() bool {
	return res.Err == nil
}

// String returns the combined output.
func (res *Result) String() string {
	return string(res.Stdout) + string(res.Stderr)
}
// WriteFileAtomic writes data to a temporary file then renames it for atomicity.
func (r *Runner) WriteFileAtomic(path string, data []byte) error {
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(r.Dir, path)
	}

	dir := filepath.Dir(abs)
	tmp, err := os.CreateTemp(dir, "go-refactor-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	if err := os.Rename(tmp.Name(), abs); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
