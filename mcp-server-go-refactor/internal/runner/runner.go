package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner provides a hardened interface for Go command execution.
type Runner struct {
	Dir          string // Execution working directory
	SlogLevel    string // slog level if needed
	GoBinaryPath string // Absolute path to the provisioned Go binary
}

// Result captures the result of a Go command.
type Result struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// StreamResult captures a streaming result of a Go command.
type StreamResult struct {
	Stdout io.ReadCloser
	Stderr *bytes.Buffer
	Cmd    *exec.Cmd
	Cancel context.CancelFunc
}

// Wait waits for the command to finish and returns the error.
func (s *StreamResult) Wait() error {
	defer s.Cancel()
	return s.Cmd.Wait()
}

// DefaultGoBinary holds the absolute path to the provisioned Go binary.
// It is updated by EnsureToolchain on startup.
var DefaultGoBinary = "go"

// New creates a new Go runner from a directory.
func New(dir string) *Runner {
	return &Runner{Dir: dir, GoBinaryPath: DefaultGoBinary}
}

// RunGo executes a 'go' command from the configured directory.
func (r *Runner) RunGo(ctx context.Context, subCmd string, args ...string) (*Result, error) {
	tctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cleanArgs := r.cleanArgs(subCmd, args)
	cmd := exec.CommandContext(tctx, r.GoBinaryPath, cleanArgs...)
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

// RunGoStream initiates a 'go' command and returns a streamable result.
// The caller is responsible for calling Wait() to release resources.
func (r *Runner) RunGoStream(ctx context.Context, subCmd string, args ...string) (*StreamResult, error) {
	tctx, cancel := context.WithTimeout(ctx, 300*time.Second) // Longer timeout for streaming

	cleanArgs := r.cleanArgs(subCmd, args)
	cmd := exec.CommandContext(tctx, r.GoBinaryPath, cleanArgs...)
	cmd.Dir = r.Dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	return &StreamResult{
		Stdout: stdout,
		Stderr: &stderr,
		Cmd:    cmd,
		Cancel: cancel,
	}, nil
}

func (r *Runner) cleanArgs(subCmd string, args []string) []string {
	clean := []string{subCmd}
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			clean = append(clean, arg)
		}
	}
	return clean
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
