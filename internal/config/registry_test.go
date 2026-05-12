package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestValidConfigKeysCoversTemplate walks the DefaultConfigTemplate YAML tree
// and asserts every leaf scalar key has a corresponding entry in ValidConfigKeys.
// This is the sync guard: if someone adds a new config parameter to the template
// but forgets to add it to the registry, this test fails at CI time.
func TestValidConfigKeysCoversTemplate(t *testing.T) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(DefaultConfigTemplate), &root); err != nil {
		t.Fatalf("Failed to parse DefaultConfigTemplate: %v", err)
	}

	if len(root.Content) == 0 {
		t.Fatal("DefaultConfigTemplate produced an empty document")
	}

	leafKeys := collectLeafKeys(root.Content[0], "")

	for _, key := range leafKeys {
		if _, ok := LookupKey(key); !ok {
			t.Errorf("Config key %q exists in DefaultConfigTemplate but is MISSING from ValidConfigKeys registry. "+
				"You must add it to ValidConfigKeys in registry.go.", key)
		}
	}
}

// TestRegistryKeysExistInTemplate verifies the reverse: every key in the registry
// actually exists as a leaf in the template. Catches stale registry entries.
func TestRegistryKeysExistInTemplate(t *testing.T) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(DefaultConfigTemplate), &root); err != nil {
		t.Fatalf("Failed to parse DefaultConfigTemplate: %v", err)
	}

	if len(root.Content) == 0 {
		t.Fatal("DefaultConfigTemplate produced an empty document")
	}

	leafKeys := collectLeafKeys(root.Content[0], "")
	leafSet := make(map[string]bool, len(leafKeys))
	for _, k := range leafKeys {
		leafSet[k] = true
	}

	for _, info := range ValidConfigKeys {
		if !leafSet[info.Key] {
			t.Errorf("Registry key %q exists in ValidConfigKeys but is MISSING from DefaultConfigTemplate. "+
				"Either add it to the template or remove it from registry.go.", info.Key)
		}
	}
}

// TestLookupKey verifies basic registry lookup behavior.
func TestLookupKey(t *testing.T) {
	// Known key
	info, ok := LookupKey("confluence.url")
	if !ok {
		t.Fatal("Expected LookupKey to find confluence.url")
	}
	if info.ValueType != "string" {
		t.Errorf("Expected ValueType 'string' for confluence.url, got %q", info.ValueType)
	}

	// Test existing key
	k, found := LookupKey("jira.disable")
	if !found {
		t.Error("Expected LookupKey to find jira.disable")
	}
	if k.ValueType != "bool" {
		t.Errorf("Expected ValueType 'bool' for jira.disable, got %q", k.ValueType)
	}

	// Unknown key
	_, ok = LookupKey("does.not.exist")
	if ok {
		t.Error("Expected LookupKey to return false for unknown key")
	}

	// Token keys must NOT be in registry (vault-only)
	for _, tokenKey := range []string{"confluence.api_key", "jira.api_key", "git.token"} {
		if _, ok := LookupKey(tokenKey); ok {
			t.Errorf("Token key %q must NOT be in ValidConfigKeys (vault-only)", tokenKey)
		}
	}
}

// TestValidKeyNames ensures the helper returns all keys.
func TestValidKeyNames(t *testing.T) {
	names := ValidKeyNames()
	if len(names) != len(ValidConfigKeys) {
		t.Errorf("ValidKeyNames returned %d names, expected %d", len(names), len(ValidConfigKeys))
	}
}

// collectLeafKeys recursively walks a YAML MappingNode tree and collects all
// dotted paths to leaf ScalarNode values. Sequence nodes (lists) are skipped.
func collectLeafKeys(node *yaml.Node, prefix string) []string {
	var keys []string

	if node.Kind != yaml.MappingNode {
		return keys
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		fullKey := keyNode.Value
		if prefix != "" {
			fullKey = prefix + "." + keyNode.Value
		}

		switch valNode.Kind {
		case yaml.ScalarNode:
			keys = append(keys, fullKey)
		case yaml.MappingNode:
			keys = append(keys, collectLeafKeys(valNode, fullKey)...)
		case yaml.SequenceNode:
			// Skip list values (e.g. standards.node URLs) — not editable via update_config
		}
	}

	return keys
}

// TestValidKeyNamesString verifies the comma-separated output.
func TestValidKeyNamesString(t *testing.T) {
	result := ValidKeyNamesString()
	if !strings.Contains(result, "confluence.url") {
		t.Error("ValidKeyNamesString should contain confluence.url")
	}
	if !strings.Contains(result, "server.log_level") {
		t.Error("ValidKeyNamesString should contain server.log_level")
	}
}
