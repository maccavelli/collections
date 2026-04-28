package state

import (
	"context"
	"sync"
	"time"

	"mcp-server-brainstorm/internal/models"
)

// Manager handles the lifecycle of brainstorming sessions
// using an in-memory store keyed by project root.
type Manager struct {
	ProjectRoot string
	mu          sync.RWMutex
	sessions    map[string]*models.Session
}

// NewManager creates a Manager rooted at the given path.
func NewManager(root string) *Manager {
	return &Manager{
		ProjectRoot: root,
		sessions:    make(map[string]*models.Session),
	}
}

// LoadSession retrieves the session for the current project root.
// If no session exists, a fresh one is created in-memory.
func (m *Manager) LoadSession(ctx context.Context) (
	*models.Session, error,
) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.RLock()
	if s, ok := m.sessions[m.ProjectRoot]; ok {
		m.mu.RUnlock()
		return s, nil
	}
	m.mu.RUnlock()

	// Create a fresh session.
	s := &models.Session{
		ProjectRoot: m.ProjectRoot,
		Status:      "DISCOVERY",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Gaps:        []models.Gap{},
		Metadata:    make(map[string]any),
	}

	m.mu.Lock()
	m.sessions[m.ProjectRoot] = s
	m.mu.Unlock()

	return s, nil
}

// SaveSession stores the session state in-memory.
func (m *Manager) SaveSession(
	ctx context.Context, session *models.Session,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	session.UpdatedAt = time.Now()

	m.mu.Lock()
	m.sessions[m.ProjectRoot] = session
	m.mu.Unlock()

	return nil
}
