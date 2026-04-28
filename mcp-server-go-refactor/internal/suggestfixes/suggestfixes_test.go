package suggestfixes

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSuggestFixes_Empty(t *testing.T) {
	defer func() { recover() }()
	tool := &Tool{}
	tool.Handle(context.Background(), &mcp.CallToolRequest{}, SuggestFixesInput{})
}
