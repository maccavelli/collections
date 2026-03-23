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

	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/handler/decision"
	"mcp-server-brainstorm/internal/handler/design"
	"mcp-server-brainstorm/internal/handler/discovery"
	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	buffer := &system.LogBuffer{}
	setupLogging(buffer)

	slog.Info("starting "+config.Name, "version", Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, buffer); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
}

func setupLogging(buffer *system.LogBuffer) {
	mw := io.MultiWriter(os.Stderr, buffer)
	handler := slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}

func run(ctx context.Context, buffer *system.LogBuffer) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	eng := engine.NewEngine(wd)
	mgr := state.NewManager(wd)

	// Register tools into global registry
	discovery.Register(mgr, eng)
	design.Register(eng)
	decision.Register(eng)
	system.Register(buffer)

	s := server.NewMCPServer(
		config.Platform+" Socratic Explorer",
		Version,
		server.WithLogging(),
	)

	// Register tools from global registry
	for _, t := range registry.Global.List() {
		s.AddTool(t.Metadata(), t.Handle)
	}

	// Register resources
	s.AddResource(mcp.NewResource("brainstorm://logs", "Active server logs",
		mcp.WithResourceDescription("Active server logs"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "brainstorm://logs",
				Text:     buffer.String(),
				MIMEType: "text/plain",
			},
		}, nil
	})

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return server.ServeStdio(s)
}
