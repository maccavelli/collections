// Package server provides functionality for the server subsystem.
package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer defines the MCPServer structure.
type MCPServer struct {
	mcpServer *mcp.Server
	session   *mcp.ServerSession
}

// NewMCPServer performs the NewMCPServer operation.
func NewMCPServer(name, version string, logger *slog.Logger) *MCPServer {
	return &MCPServer{
		mcpServer: mcp.NewServer(
			&mcp.Implementation{Name: name, Version: version},
			&mcp.ServerOptions{Logger: logger},
		),
	}
}

// MCPServer performs the MCPServer operation.
func (s *MCPServer) MCPServer() *mcp.Server {
	return s.mcpServer
}

// Serve performs the Serve operation.
func (s *MCPServer) Serve(ctx context.Context, stdout io.WriteCloser, reader io.ReadCloser) error {
	t := &mcp.IOTransport{
		Reader: reader,
		Writer: stdout,
	}
	session, err := s.mcpServer.Connect(ctx, t, nil)
	if err == nil {
		s.session = session
	}
	return err
}

// Session performs the Session operation.
func (s *MCPServer) Session() *mcp.ServerSession {
	return s.session
}
