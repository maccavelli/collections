package git

import (
	"context"

	"mcp-server-magicdev/internal/db"
)

// Provider defines the standard interface for interacting with a Git provider (e.g., GitHub, GitLab).
type Provider interface {
	PushDocuments(ctx context.Context, store *db.Store, jiraID, targetBranch, title string, fileContent, adrContent []byte, bp *db.Blueprint) error
}
