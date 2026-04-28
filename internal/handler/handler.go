package handler

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"mcp-server-magicskills/internal/config"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/external"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LogBuffer implements io.Writer with ring-buffer semantics for slog
type LogBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

var secretRegex = regexp.MustCompile(config.ResolveRedactionPattern())

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	redacted := secretRegex.ReplaceAll(p, []byte("[REDACTED]"))
	_, err = lb.buf.Write(redacted)
	if err != nil {
		return len(p), err
	}
	n = len(p)

	if lb.buf.Len() > config.LogBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - config.LogTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		_, err = lb.buf.Write(newData)
		if err != nil {
			return 0, fmt.Errorf("failed to rewrite buffer: %w", err)
		}
	}
	return n, nil
}

func (lb *LogBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buf.String()
}

// MagicSkillsHandler provides shared resources for MagicSkills tools and resources.
type MagicSkillsHandler struct {
	Engine       *engine.Engine
	Logs         *LogBuffer
	RecallClient *external.MCPClient
}

// HandleReadResource handles dashboard and log resource requests.
func (h *MagicSkillsHandler) HandleReadResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	switch request.Params.URI {
	case "magicskills://status":
		var b strings.Builder
		b.Grow(512)
		b.WriteString("# MagicSkills Dashboard\n\n")
		b.WriteString(fmt.Sprintf("Total Skills Indexed: %d\n\n", h.Engine.SkillCount()))
		b.WriteString("## Available Skills\n")

		for s := range h.Engine.AllSkills() {
			b.WriteString(fmt.Sprintf("- **%s** (v%s)\n", s.Metadata.Name, s.Metadata.Version))
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      request.Params.URI,
					MIMEType: "text/markdown",
					Text:     b.String(),
				},
			},
		}, nil
	case "magicskills://logs":
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      request.Params.URI,
					MIMEType: "text/plain",
					Text:     h.Logs.String(),
				},
			},
		}, nil
	default:
		return nil, mcp.ResourceNotFoundError(request.Params.URI)
	}
}

// RegisterResources registers static resources with the server.
func (h *MagicSkillsHandler) RegisterResources(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		Name:        "Skill Status Dashboard",
		URI:         "magicskills://status",
		Description: "Overview of currently indexed and active skills.",
		MIMEType:    "text/markdown",
	}, h.HandleReadResource)

	s.AddResource(&mcp.Resource{
		Name:        "Internal Logs",
		URI:         "magicskills://logs",
		Description: "Internal server logs for monitoring.",
		MIMEType:    "text/plain",
	}, h.HandleReadResource)
}
