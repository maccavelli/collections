package config

import (
	"testing"
	"time"
)

func TestConfig_Constants(t *testing.T) {
	if Name != "mcp-server-duckduckgo" {
		t.Error("bad name")
	}
	if Platform != "DuckDuckGo" {
		t.Error("bad platform")
	}
	if DefaultTimeout != 15*time.Second {
		t.Error("bad timeout")
	}
	if MaxBodyBytes != 10*1024*1024 {
		t.Error("bad max body size")
	}
	if MaxSnippetLength != 1000 {
		t.Error("bad snippet max")
	}
	if VQDCacheLimit != 500 {
		t.Error("bad vqd cache limit")
	}
}
