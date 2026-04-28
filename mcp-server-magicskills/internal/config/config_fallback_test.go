package config

import (
	"testing"
)

func TestConfigResolvers_Fallback(t *testing.T) {
	// Execute native branch checks without OS mocks
	_ = ResolveRoots()
	_ = ResolveDataDir()
	_ = ResolveRedactionPattern()
}
