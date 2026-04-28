package models

import "time"

// SkillMetadata represents the frontmatter of a SKILL.md
type SkillMetadata struct {
	Name          string   `yaml:"name" json:"name"`
	Description   string   `yaml:"description" json:"description"`
	Persona       string   `yaml:"persona" json:"persona,omitempty"`
	Tags          []string `yaml:"tags" json:"tags,omitempty"`
	Version       string   `yaml:"version" json:"version"`
	ContextDomain string   `yaml:"domain" json:"domain,omitempty"` // e.g. "go", "python", "devops"
	Requirements  []string `yaml:"requirements" json:"requirements,omitempty"`
}

// Skill represents a fully parsed skill including its metadata and content sections
type Skill struct {
	Metadata        SkillMetadata     `json:"metadata"`
	Sections        map[string]string `json:"sections"` // e.g., "Workflow", "Best Practices"
	RawPath         string            `json:"raw_path"`
	Hash            string            `json:"hash"`   // SHA-256 of the raw file content
	Digest          string            `json:"digest"` // Densely formatted markdown for agent context
	EstimatedTokens int               `json:"estimated_tokens"`
	SchemaVersion   string            `json:"schema_version"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
