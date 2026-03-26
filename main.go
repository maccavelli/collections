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

	"mcp-server-go-refactor/internal/config"
	"mcp-server-go-refactor/internal/handler"
	"mcp-server-go-refactor/internal/handler/system"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the build version of the application
var Version = "3.1.4"

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
	h := slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(h))
}

func run(ctx context.Context, buffer *system.LogBuffer) error {
	// Register all tools explicitly
	handler.RegisterAllTools(buffer)

	// Create official MCP server
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    config.Platform + " Analyzer",
			Version: Version,
		},
		&mcp.ServerOptions{},
	)

	// Load registered tools into the server
	handler.LoadToolsFromRegistry(s)

	// Register resources
	s.AddResource(&mcp.Resource{
		URI:         "go-refactor://logs",
		Name:        "Active server logs",
		Description: "Active server logs",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "go-refactor://logs",
					MIMEType: "text/plain",
					Text:     buffer.String(),
				},
			},
		}, nil
	})

	return s.Run(ctx, &mcp.StdioTransport{})
}

func printVersion() {
	fmt.Printf("mcp-server-go-refactor version %s\n", Version)
}
