package config

import (
	"encoding/json"
	"testing"
)

func TestConfig_MapBehavior(t *testing.T) {
	// Test if accessing a missing key in the map and then modifying the return value
	// actually modifies the map (it shouldn't for value types like ProviderConfig).
	conf := &Config{
		Providers: make(map[string]ProviderConfig),
	}

	provider := "anthropic"
	pc := conf.Providers[provider] // Missing key, returns zero value ProviderConfig{}
	
	pc.APIKey = "test-key"
	pc.Model = "test-model"
	
	// At this point, conf.Providers[provider] is still the zero value if we didn't assign it back.
	if _, ok := conf.Providers[provider]; ok {
		t.Errorf("Expected provider to be missing from map before assignment")
	}

	conf.ActiveProvider = provider
	conf.Providers[provider] = pc // Explicit assignment back to the map

	if val, ok := conf.Providers[provider]; !ok || val.APIKey != "test-key" {
		t.Errorf("Expected provider to be present after assignment")
	}

	data, _ := json.Marshal(conf)
	t.Logf("JSON: %s", string(data))
}
