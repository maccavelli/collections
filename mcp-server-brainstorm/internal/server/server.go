package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPServer struct {
	mcpServer *mcp.Server
	session   *mcp.ServerSession
}

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
	session, err := s.mcpServer.Connect(ctx, t, nil)
	if err == nil {
		s.session = session
	}
	return err
}

func (s *MCPServer) Session() *mcp.ServerSession {
	return s.session
}
