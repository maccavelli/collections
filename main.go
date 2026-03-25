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
	"mcp-server-brainstorm/internal/config"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/handler/decision"
	"mcp-server-brainstorm/internal/handler/design"
	"mcp-server-brainstorm/internal/handler/discovery"
	"mcp-server-brainstorm/internal/handler/system"
	"mcp-server-brainstorm/internal/registry"
	"mcp-server-brainstorm/internal/state"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	buffer := &system.LogBuffer{}
	logger := setupLogging(buffer)

	slog.Info("starting "+config.Name, "version", Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, buffer, logger); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
}

func setupLogging(buffer *system.LogBuffer) *slog.Logger {
	mw := io.MultiWriter(os.Stderr, buffer)
	logger := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	return logger
}

func run(ctx context.Context, buffer *system.LogBuffer, logger *slog.Logger) error {
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

	// Setup MCP Server using official SDK
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{
			Name:    config.Platform + " Socratic Explorer",
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

	// Register resources
	mcpSrv.AddResource(&mcp.Resource{
		Name:        "Active server logs",
		URI:         "brainstorm://logs",
		Description: "Active server logs for auditing AI decision-making steps.",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      request.Params.URI,
					Text:     buffer.String(),
					MIMEType: "text/plain",
				},
			},
		}, nil
	})

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}
