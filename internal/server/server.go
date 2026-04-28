package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the base Go SDK MCP Server to provide a customized
// stdio transport initialization and capability registration workflow.
// It acts as the primary lifecycle manager for the MCP connection.
type MCPServer struct {
	mcpServer *mcp.Server
}

// NewMCPServer initializes a new MCPServer instance with the designated name,
// version, and structured logger. It configures the underlying SDK server options.
func NewMCPServer(name, version string, logger *slog.Logger) *MCPServer {
	return &MCPServer{
		mcpServer: mcp.NewServer(
			&mcp.Implementation{Name: name, Version: version},
			&mcp.ServerOptions{Logger: logger},
		),
	}
}

func (s *MCPServer) MCPServer() *mcp.Server {
	return s.mcpServer
}

func (s *MCPServer) Serve(ctx context.Context, stdout io.WriteCloser, reader io.ReadCloser) error {
	t := &mcp.IOTransport{
		Reader: reader,
		Writer: stdout,
	}
	_, err := s.mcpServer.Connect(ctx, t, nil)
	return err
}
