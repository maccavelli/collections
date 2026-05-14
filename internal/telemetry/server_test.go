package telemetry_test

import (
	"testing"

	"mcp-server-socratic-thinker/internal/telemetry"
)

func TestServer_NilSafety(t *testing.T) {
	// Verify nil receiver methods don't panic
	var s *telemetry.Server
	s.Start()                              // should not panic
	s.Broadcast(telemetry.MetricPayload{}) // should not panic
	s.Close()                              // should not panic
}

func TestServer_NewServer_Binds(t *testing.T) {
	// NewServer should bind to the first available port
	s := telemetry.NewServer()
	if s == nil {
		t.Skip("could not bind to any telemetry port (ports in use)")
	}
	defer s.Close()

	// Verify Start doesn't panic
	s.Start()

	// Verify Broadcast with no connected dashboard doesn't panic
	s.Broadcast(telemetry.MetricPayload{
		UptimeSeconds:    42,
		MemoryAllocBytes: 1024,
		ActiveGoroutines: 5,
	})
}
