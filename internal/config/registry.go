// Package config provides functionality for the config subsystem.
package config

import "strings"

// ConfigKeyInfo describes a single editable configuration parameter.
type ConfigKeyInfo struct {
	Key         string // Dotted path, e.g. "jira.url"
	Description string // Human-readable description for MCP schema
	ValueType   string // "string" or "bool"
}

// ValidConfigKeys enumerates ALL scalar config parameters editable via
// the update_config MCP tool. When adding a new parameter to
// DefaultConfigTemplate in config.go, you MUST add a corresponding entry
// here — the sync guard test in registry_test.go enforces this.
var ValidConfigKeys = []ConfigKeyInfo{
	// Confluence
	{Key: "confluence.url", Description: "Confluence instance base URL", ValueType: "string"},
	{Key: "confluence.space", Description: "Confluence space key for publishing", ValueType: "string"},
	{Key: "confluence.parent_page_id", Description: "Parent page ID for nesting documents", ValueType: "string"},
	{Key: "confluence.disable", Description: "Disable Confluence layer (true/false)", ValueType: "bool"},

	// Jira
	{Key: "jira.email", Description: "Jira authentication email", ValueType: "string"},
	{Key: "jira.url", Description: "Jira instance base URL", ValueType: "string"},
	{Key: "jira.project", Description: "Jira project key for issue creation", ValueType: "string"},
	{Key: "jira.issue", Description: "Existing Jira issue key to attach documents to", ValueType: "string"},
	{Key: "jira.disable", Description: "Disable Jira layer (true/false)", ValueType: "bool"},
	{Key: "jira.story_points_field", Description: "Custom field ID for story points estimation", ValueType: "string"},

	// GitLab
	{Key: "gitlab.server_url", Description: "GitLab server base URL", ValueType: "string"},
	{Key: "gitlab.project_path", Description: "GitLab namespace/project path", ValueType: "string"},
	{Key: "gitlab.default_branch", Description: "Default target branch for artifact pushes", ValueType: "string"},
	{Key: "gitlab.disable", Description: "Disable GitLab integration (true/false)", ValueType: "bool"},

	// GitHub
	{Key: "github.repository", Description: "GitHub repository path (e.g. owner/repo)", ValueType: "string"},
	{Key: "github.default_branch", Description: "Default target branch for artifact pushes", ValueType: "string"},
	{Key: "github.disable", Description: "Disable GitHub integration (true/false)", ValueType: "bool"},

	// Agent
	{Key: "agent.default_stack", Description: "Default technology stack (.NET, Node, Go, Python)", ValueType: "string"},

	// Runtime
	{Key: "runtime.gomemlimit", Description: "Go GC soft memory limit (e.g. 4GB, 512MB)", ValueType: "string"},
	{Key: "runtime.gomaxprocs", Description: "Max OS threads for Go runtime", ValueType: "string"},

	// Server
	{Key: "server.log_level", Description: "Minimum log severity (DEBUG, INFO, WARN, ERROR)", ValueType: "string"},
	{Key: "server.db_path", Description: "Absolute path to BuntDB session file", ValueType: "string"},

	// LLM
	{Key: "llm.provider", Description: "LLM provider (gemini, openai, claude)", ValueType: "string"},
	{Key: "llm.model", Description: "The chosen model used by the Intelligence Engine", ValueType: "string"},
	{Key: "llm.disable", Description: "Disable the LLM Intelligence Engine (true/false)", ValueType: "bool"},

	// Standards Constraints
	{Key: "standards.node.path", Description: "Filesystem path to standard Node.js templates", ValueType: "string"},
	{Key: "standards.node.total_files", Description: "Max allowed files constraint for Node.js", ValueType: "string"},
	{Key: "standards.node.max_directory_depth", Description: "Max allowed directory depth constraint for Node.js", ValueType: "string"},
	{Key: "standards.dotnet.path", Description: "Filesystem path to standard .NET templates", ValueType: "string"},
	{Key: "standards.dotnet.total_files", Description: "Max allowed files constraint for .NET", ValueType: "string"},
	{Key: "standards.dotnet.max_directory_depth", Description: "Max allowed directory depth constraint for .NET", ValueType: "string"},
}

// LookupKey searches ValidConfigKeys for the given dotted key path.
func LookupKey(key string) (ConfigKeyInfo, bool) {
	for _, k := range ValidConfigKeys {
		if k.Key == key {
			return k, true
		}
	}
	return ConfigKeyInfo{}, false
}

// ValidKeyNames returns a list of all valid key paths for schema hints.
func ValidKeyNames() []string {
	names := make([]string, len(ValidConfigKeys))
	for i, k := range ValidConfigKeys {
		names[i] = k.Key
	}
	return names
}

// ValidKeyNamesString returns a comma-separated string of all valid key paths.
func ValidKeyNamesString() string {
	return strings.Join(ValidKeyNames(), ", ")
}
