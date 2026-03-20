package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"mcp-server-brainstorm/internal/models"
)

const StateFile = ".brainstorm.json"

// Manager handles the lifecycle of the brainstorming
// state.
type Manager struct {
	ProjectRoot string
}

// NewManager creates a Manager rooted at the given path.
func NewManager(root string) *Manager {
	return &Manager{ProjectRoot: root}
}

// LoadSession reads the state file from the project root.
func (m *Manager) LoadSession(ctx context.Context) (
	*models.Session, error,
) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	path := filepath.Join(m.ProjectRoot, StateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.Session{
				ProjectRoot: m.ProjectRoot,
				Status:      "DISCOVERY",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Gaps:        []models.Gap{},
			}, nil
		}
		return nil, fmt.Errorf(
			"failed to read state file: %w", err,
		)
	}

	var session models.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf(
			"failed to parse session: %w", err,
		)
	}

	return &session, nil
}

// SaveSession writes the session state to the project
// root using atomic write-then-rename to prevent
// corruption from interrupted writes.
func (m *Manager) SaveSession(
	ctx context.Context, session *models.Session,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf(
			"failed to marshal session: %w", err,
		)
	}

	finalPath := filepath.Join(
		m.ProjectRoot, StateFile,
	)
	tmpPath := finalPath + ".tmp"

	if err := os.WriteFile(
		tmpPath, data, 0644,
	); err != nil {
		return fmt.Errorf(
			"failed to write temp state file: %w", err,
		)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		// Best-effort cleanup of the temp file.
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			slog.Debug("failed to cleanup temp state file",
				"path", tmpPath, "error", removeErr)
		}
		return fmt.Errorf(
			"failed to rename state file: %w", err,
		)
	}

	return nil
}
