// Package main is the entry point for the Brainstorm
// Socratic Explorer MCP server. It registers tools and
// serves via stdio.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/handler"
	"mcp-server-brainstorm/internal/state"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// logBufferLimit is the maximum size of the in-memory
// log buffer before trimming.
const logBufferLimit = 1024 * 1024 // 1MB

// logTrimTarget is the size to trim back to when the
// buffer exceeds the limit (keeps last ~512KB).
const logTrimTarget = 512 * 1024

// LogBuffer stores recent server logs in memory with
// ring-buffer semantics.
type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n, err = lb.buf.Write(p)
	if err != nil {
		return n, err
	}

	if lb.buf.Len() > logBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - logTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		// Efficiently trim by creating a new buffer from the slice
		// without full reset/rewrite if possible, but Buffer doesn't
		// support this easily. We'll use a new buffer to minimize
		// fragmentation if this happens often.
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		if _, err := lb.buf.Write(newData); err != nil {
			// This is extremely unlikely for a bytes.Buffer, but
			// handle it for production safety.
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return n, nil
}

func (lb *LogBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.String()
}

var globalLogBuffer = &LogBuffer{}

func main() {
	mw := io.MultiWriter(os.Stderr, globalLogBuffer)
	logger := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("fatal server error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	s := server.NewMCPServer("Brainstorm Socratic Explorer", Version, server.WithLogging())
	mgr := state.NewManager(wd)
	eng := engine.NewEngine(wd)

	registerDiscoveryTools(s, mgr, eng)
	registerDesignTools(s, eng)
	registerDecisionTools(s, eng)

	slog.Info("server starting", "version", Version, "root", wd)

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server", "signal", ctx.Err())
	}()

	return server.ServeStdio(s)
}

func registerDiscoveryTools(s *server.MCPServer, mgr *state.Manager, eng *engine.Engine) {
	s.AddTool(mcp.NewTool("discover_project",
		mcp.WithDescription("Performs a unified discovery scan, identifying gaps and suggesting the next logical step."),
		mcp.WithString("path", mcp.Description("Optional absolute path to the project root.")),
	), handler.HandleDiscoverProject(mgr, eng))

	s.AddTool(mcp.NewTool("get_internal_logs",
		mcp.WithDescription("Retrieves recent internal server logs."),
		mcp.WithNumber("max_lines", mcp.Description("Max log lines to return (default 50).")),
	), handler.HandleGetInternalLogs(globalLogBuffer))
}

func registerDesignTools(s *server.MCPServer, eng *engine.Engine) {
	s.AddTool(mcp.NewTool("critique_design",
		mcp.WithDescription("Provides a consolidated, multi-dimensional assessment of a design (Socratic, Red Team, Quality)."),
		mcp.WithString("design", mcp.Description("The design text to critique"), mcp.Required()),
	), handler.HandleCritiqueDesign(eng))

	s.AddTool(mcp.NewTool("analyze_evolution",
		mcp.WithDescription("Identifies risks in proposed project changes."),
		mcp.WithString("proposal", mcp.Description("The proposed change or extension"), mcp.Required()),
	), handler.HandleAnalyzeEvolution(eng))
}

func registerDecisionTools(s *server.MCPServer, eng *engine.Engine) {
	s.AddTool(mcp.NewTool("capture_decision_logic",
		mcp.WithDescription("Generates a structured ADR capturing context and alternatives."),
		mcp.WithString("decision", mcp.Description("The decision being made"), mcp.Required()),
		mcp.WithString("alternatives", mcp.Description("The considered alternatives"), mcp.Required()),
	), handler.HandleCaptureDecision(eng))
}
