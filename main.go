package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magicskills/internal/config"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/handler/bootstrap"
	"mcp-server-magicskills/internal/handler/discovery"
	"mcp-server-magicskills/internal/handler/retrieval"
	"mcp-server-magicskills/internal/handler/system"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/scanner"
)

// Version is the build version of the application.
var Version = "3.1.2"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mcp-server-magicskills version %s\n", Version)
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := execute(ctx); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context) error {
	logBuffer := &handler.LogBuffer{}
	logger := setupLogging(logBuffer)

	// Phase 1: Configuration & Discovery
	roots := config.ResolveRoots()
	roots = append(roots, scanner.FindProjectSkillsRoots()...)

	eng := engine.NewEngine()
	h := &handler.MagicSkillsHandler{
		Engine: eng,
		Logs:   logBuffer,
	}

	// Add manual roots from args
	for _, arg := range flag.Args() {
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			roots = append(roots, arg)
			slog.Info("added manual skill root", "path", arg)
		}
	}

	scn, err := scanner.NewScanner(roots)
	if err != nil {
		return fmt.Errorf("scanner init: %w", err)
	}
	defer scn.Watcher.Close()

	// Tool Registration via granular packages
	discovery.Register(eng)
	retrieval.Register(eng)
	bootstrap.Register(eng)
	system.Register(eng, scn)

	// Initial Ingestion (Context Aware)
	files, err := scn.Discover(ctx)
	if err != nil {
		slog.Warn("discovery produced errors", "error", err)
	}
	if err := eng.Ingest(ctx, files); err != nil {
		slog.Error("initial ingestion failed", "error", err)
	}
	slog.Info("engine ready", "skillsCount", len(eng.Skills), "version", Version, "rootsCount", len(roots))

	// Setup MCP Server
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-server-magicskills",
			Version: Version,
		},
		&mcp.ServerOptions{
			Logger: logger,
		},
	)

	// Register tools from global registry
	for _, t := range registry.Global.List() {
		t.Register(mcpSrv)
	}

	// Register Static Resources
	h.RegisterResources(mcpSrv)

	// Background Incremental Watcher
	scn.Listen(ctx, func(path string) {
		if err := eng.IngestSingle(ctx, path); err != nil {
			slog.Error("incremental update failed", "path", path, "error", err)
		} else {
			slog.Info("engine cache updated", "path", path)
		}
	}, func(path string) {
		eng.Remove(ctx, path)
		slog.Info("engine cache item removed", "path", path)
	})

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

func setupLogging(logBuffer *handler.LogBuffer) *slog.Logger {
	mw := io.MultiWriter(os.Stderr, logBuffer)
	logger := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	return logger
}

