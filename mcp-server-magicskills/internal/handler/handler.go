package handler

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
)

const (
	logBufferLimit = 1024 * 512 // 512KB
	logTrimTarget  = 256 * 1024 // 256KB
)

// LogBuffer implements io.Writer with ring-buffer semantics for slog
type LogBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n, err = lb.buf.Write(p)
	if err != nil {
		return n, err
	}

	if lb.buf.Len() > logBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - logTrimTarget
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
	Engine *engine.Engine
	Logs   *LogBuffer
}

// HandleReadResource handles dashboard and log resource requests.
func (h *MagicSkillsHandler) HandleReadResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	switch request.Params.URI {
	case "magicskills://status":
		var b strings.Builder
		b.Grow(512)
		b.WriteString("# MagicSkills Dashboard\n\n")
		b.WriteString(fmt.Sprintf("Total Skills Indexed: %d\n\n", len(h.Engine.Skills)))
		b.WriteString("## Available Skills\n")

		for s := range h.Engine.AllSkills() {
			b.WriteString(fmt.Sprintf("- **%s** (v%s)\n", s.Metadata.Name, s.Metadata.Version))
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		}, nil
	case "magicskills://logs":
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/plain",
				Text:     h.Logs.String(),
			},
		}, nil
	default:
		return nil, fmt.Errorf("resource not found: %s", request.Params.URI)
	}
}

// HandleListResources lists available static resources.
func (h *MagicSkillsHandler) HandleListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	resources := []mcp.Resource{
		mcp.NewResource("magicskills://status", "Skill Status Dashboard",
			mcp.WithResourceDescription("Overview of currently indexed and active skills."),
			mcp.WithMIMEType("text/markdown"),
		),
		mcp.NewResource("magicskills://logs", "Internal Logs",
			mcp.WithResourceDescription("Internal server logs for monitoring."),
			mcp.WithMIMEType("text/plain"),
		),
	}
	return &mcp.ListResourcesResult{Resources: resources}, nil
}
