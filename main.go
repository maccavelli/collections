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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Version is the build version of the application
var Version = "dev"

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
	// Register all tools explicitly
	handler.RegisterAllTools(buffer)

	s := server.NewMCPServer(
		config.Platform+" Analyzer",
		Version,
		server.WithLogging(),
	)

	// Load registered tools into the server
	handler.LoadToolsFromRegistry(s)

	// Register resources
	s.AddResource(mcp.NewResource("go-refactor://logs", "Active server logs",
		mcp.WithResourceDescription("Active server logs"),
		mcp.WithMIMEType("text/plain"),
	), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "go-refactor://logs",
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

func printVersion() {
	fmt.Printf("mcp-server-go-refactor version %s\n", Version)
}
