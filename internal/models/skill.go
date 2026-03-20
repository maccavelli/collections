package models

// SkillMetadata represents the frontmatter of a SKILL.md
type SkillMetadata struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Persona     string   `yaml:"persona" json:"persona,omitempty"`
	Tags        []string `yaml:"tags" json:"tags,omitempty"`
	Version     string   `yaml:"version" json:"version"`
}

// Skill represents a fully parsed skill including its metadata and content sections
type Skill struct {
	Metadata SkillMetadata     `json:"metadata"`
	Sections map[string]string `json:"sections"` // e.g., "Workflow", "Best Practices"
	RawPath  string            `json:"raw_path"`
}
