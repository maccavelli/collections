package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the official Go SDK implementation.
type MCPServer struct {
	mcpServer *mcp.Server
}

// NewMCPServer initializes a server implementation with basic meta information.
func NewMCPServer(name, version string, logger *slog.Logger) *MCPServer {
	return &MCPServer{
		mcpServer: mcp.NewServer(
			&mcp.Implementation{Name: name, Version: version},
			&mcp.ServerOptions{Logger: logger},
		),
	}
}

// MCPServer exposes the underlying SDK server instance.
func (s *MCPServer) MCPServer() *mcp.Server {
	return s.mcpServer
}

// Serve connects the IO transport via standard generic pipes.
func (s *MCPServer) Serve(ctx context.Context, stdout io.WriteCloser, reader io.ReadCloser) error {
	t := &mcp.IOTransport{
		Reader: reader,
		Writer: stdout,
	}
	_, err := s.mcpServer.Connect(ctx, t, nil)
	return err
}
