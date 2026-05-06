// Package config provides functionality for the config subsystem.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// UpdateConfigKey surgically updates a single scalar value in the magicdev.yaml
// configuration file while preserving all existing comments and structure.
func UpdateConfigKey(key, value string) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if len(root.Content) == 0 {
		return fmt.Errorf("empty yaml document")
	}

	doc := root.Content[0]
	parts := strings.Split(key, ".")

	if !updateNode(doc, parts, value) {
		return fmt.Errorf("key not found in configuration: %s", key)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("failed to encode yaml: %w", err)
	}

	return os.WriteFile(path, out, 0600)
}

func updateNode(node *yaml.Node, path []string, value string) bool {
	if len(path) == 0 {
		return false
	}

	if node.Kind == yaml.DocumentNode {
		if len(node.Content) > 0 {
			return updateNode(node.Content[0], path, value)
		}
		return false
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			if keyNode.Value == path[0] {
				if len(path) == 1 {
					if valNode.Kind == yaml.ScalarNode {
						valNode.Value = value
						return true
					}
					return false
				}
				return updateNode(valNode, path[1:], value)
			}
		}
	}
	return false
}
