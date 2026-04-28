package config

import (
	"os"
	"testing"
)

func TestConfig_Constants(t *testing.T) {
	if Name != "mcp-server-go-refactor" {
		t.Error("bad name")
	}
	if Platform != "Go Refactor" {
		t.Error("bad platform")
	}
	if CyclomaticComplexityTarget != 10 {
		t.Error("bad cyclomatic default")
	}
	if CognitiveComplexityTarget != 15 {
		t.Error("bad cognitive default")
	}
}

func TestResolveAPIURLs(t *testing.T) {
	os.Setenv("MCP_API_URL", "http://test1, http://test2 ,")
	defer os.Clearenv()

	urls := ResolveAPIURLs()
	if len(urls) != 2 {
		t.Errorf("expected 2 urls, got %d", len(urls))
	}
	if len(urls) > 0 && urls[0] != "http://test1" {
		t.Errorf("expected trimmed urls")
	}

	os.Setenv("MCP_API_URL", "")
	if urls := ResolveAPIURLs(); len(urls) != 0 {
		t.Error("expected 0 urls")
	}
}
