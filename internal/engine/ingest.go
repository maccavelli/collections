package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mcp-server-magicskills/internal/models"

	"gopkg.in/yaml.v3"
)

func extractWorkspace(p string) string {
	before, _, ok := strings.Cut(p, "/.agent/")
	if !ok {
		// Fallback to raw directory structure if standalone file
		return filepath.Dir(p)
	}
	return before
}

// SyncDir intelligently processes directory paths sequentially by calculating SHA-256 deltas.
// Net-new or modified files are ingested precisely without destroying global engine maps.
// It executes sequentially to guarantee that local project paths intentionally override global paths.
func (e *Engine) SyncDir(ctx context.Context, paths []string) (added int, updated int, deleted int, err error) {
	seenPaths := make(map[string]bool)

	for _, p := range paths {
		seenPaths[p] = true

		if ctx.Err() != nil {
			return added, updated, deleted, ctx.Err()
		}

		data, err := os.ReadFile(p)
		if err != nil {
			slog.Warn("SyncDir: failed to read file", "path", p, "error", err)
			continue
		}

		hash := hashContent(data)

		e.mu.RLock()
		slug, ok := e.PathToName[p]
		var isAdded, isUpdated bool
		if !ok {
			isAdded = true
		} else {
			skill, exists := e.Skills[slug]
			if !exists || skill.Hash != hash {
				isUpdated = true
			}
		}
		e.mu.RUnlock()

		if isAdded || isUpdated {
			if err := e.IngestSingle(ctx, p); err != nil {
				slog.Warn("SyncDir: IngestSingle failed", "path", p, "error", err)
			} else {
				if isAdded {
					added++
				} else {
					updated++
				}
			}
		}
	}

	// Calculate and remove deletions safely under read lock
	e.mu.RLock()
	var toDelete []string
	for path := range e.PathToName {
		if !seenPaths[path] && !strings.Contains(path, "#") { // ignore InjectRemoteCapability URLs
			toDelete = append(toDelete, path)
		}
	}
	e.mu.RUnlock()

	for _, p := range toDelete {
		e.Remove(ctx, p)
		deleted++
	}

	return added, updated, deleted, nil
}

func (e *Engine) IngestSingle(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("IngestSingle: failed to read file", "path", path, "error", err)
		return fmt.Errorf("read failed: %w", err)
	}

	// Parse and hash outside the lock
	hash := hashContent(data)
	skill, err := parseSkillFile(path, data)
	if err != nil {
		slog.Warn("IngestSingle: failed to parse skill", "path", path, "error", err)
		return fmt.Errorf("parse failed: %w", err)
	}
	skill.Hash = hash
	skill.Digest = GenerateDigest(skill)
	skill.UpdatedAt = time.Now()
	skill.EstimatedTokens = len(skill.Digest) / 4
	skill.SchemaVersion = CurrentSchemaVersion

	workspace := extractWorkspace(path)
	slug := slugify(skill.Metadata.Name)

	// Acquire write lock — re-check hash to handle TOCTOU races
	e.mu.Lock()
	if existing, ok := e.Skills[slug]; ok && existing.Hash == hash {
		e.mu.Unlock()
		return nil
	}
	e.Skills[slug] = skill
	e.PathToName[path] = slug
	e.mu.Unlock()

	// Index in Bleve after releasing the lock (Bleve has internal locking)
	err = e.Bleve.Index(slug, map[string]any{
		"workspace":      workspace,
		"name":           skill.Metadata.Name,
		"description":    skill.Metadata.Description,
		"context_domain": skill.Metadata.ContextDomain,
		"tags":           skill.Metadata.Tags,
		"content":        skill.Sections["full"],
		"digest":         skill.Digest,
	})
	if err != nil {
		slog.Error("bleve single index failed", "error", err)
	}
	return nil
}

func (e *Engine) Remove(ctx context.Context, path string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if name, ok := e.PathToName[path]; ok {
		delete(e.Skills, name)
		delete(e.PathToName, path)
		if err := e.Bleve.Delete(name); err != nil {
			slog.Warn("failed to remove item from bleve index", "name", name, "error", err)
		}
	}
}

// InjectRemoteCapability adds a dynamically discovered external MCP tool into the local semantic engine.
func (e *Engine) InjectRemoteCapability(ctx context.Context, apiURL, name, desc string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Use an internal slug pattern to prevent collision with local files
	slug := slugify("remote_" + name)

	skill := &models.Skill{
		Metadata: models.SkillMetadata{
			Name:          name,
			Description:   desc,
			ContextDomain: "external_api",
			Tags:          []string{"remote", "mcp_tool", "context"},
		},
		Sections: map[string]string{
			"full": desc,
		},
		RawPath:       apiURL + "#" + name,
		UpdatedAt:     time.Now(),
		SchemaVersion: CurrentSchemaVersion,
	}

	skill.Digest = GenerateDigest(skill)
	skill.EstimatedTokens = len(skill.Digest) / 4

	e.Skills[slug] = skill
	e.PathToName[skill.RawPath] = slug

	err := e.Bleve.Index(slug, map[string]any{
		"name":           name,
		"description":    desc,
		"context_domain": skill.Metadata.ContextDomain,
		"tags":           skill.Metadata.Tags,
		"content":        desc,
		"digest":         skill.Digest,
	})
	if err != nil {
		slog.Error("bleve remote index failed", "name", name, "error", err)
	}
}

func parseSkillFile(path string, data []byte) (*models.Skill, error) {
	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid skill format: missing YAML frontmatter")
	}

	var meta models.SkillMetadata
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if meta.Name == "" {
		meta.Name = filepath.Base(filepath.Dir(path))
	}

	content := strings.TrimSpace(parts[2])
	sections := parseSections(content)
	sections["full"] = content

	return &models.Skill{
		Metadata:      meta,
		Sections:      sections,
		RawPath:       path,
		SchemaVersion: CurrentSchemaVersion,
	}, nil
}

func hashContent(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
