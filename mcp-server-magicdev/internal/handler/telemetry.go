// Package handler provides functionality for the handler subsystem.
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/telemetry"
)

// RecordStepTiming calculates and records the phase dwell time for a specific step.
// It assumes that the previous step's StartedAt was already populated. If not, it just sets it.
func RecordStepTiming(session *db.SessionState, step string) {
	if session.StepTimings == nil {
		session.StepTimings = make(map[string]db.StepTiming)
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// Complete the previous step if there is one
	if session.CurrentStep != "" && session.CurrentStep != step {
		if prev, ok := session.StepTimings[session.CurrentStep]; ok {
			if prev.CompletedAt == "" {
				prev.CompletedAt = nowStr
				if startedAt, err := time.Parse(time.RFC3339, prev.StartedAt); err == nil {
					prev.DurationMs = now.Sub(startedAt).Milliseconds()
				}
				session.StepTimings[session.CurrentStep] = prev
			}
		}
	}

	// Complete the current step — RecordStepTiming is called at step completion,
	// so we mark it done immediately to prevent infinite latency counters.
	existing, exists := session.StepTimings[step]
	if !exists {
		existing = db.StepTiming{StartedAt: nowStr}
	}
	existing.CompletedAt = nowStr
	if startedAt, err := time.Parse(time.RFC3339, existing.StartedAt); err == nil {
		existing.DurationMs = now.Sub(startedAt).Milliseconds()
	}
	session.StepTimings[step] = existing
}

// LogSessionHash generates a SHA-256 hash of the session state for inter-tool integrity verification.
func LogSessionHash(session *db.SessionState, step string) {
	b, err := json.Marshal(session)
	if err != nil {
		slog.Warn("Failed to marshal session for telemetry hashing", "step", step, "error", err)
		return
	}
	hash := sha256.Sum256(b)
	hexHash := hex.EncodeToString(hash[:])
	slog.Info("Session state integrity hash",
		"step", step,
		"session_id", session.SessionID,
		"sha256", hexHash,
	)
	
	// Push directly to the high-performance memory ring
	telemetry.GlobalRing.Push(fmt.Sprintf(`{"msg":"Session state integrity hash","step":%q,"session_id":%q,"sha256":%q}`, step, session.SessionID, hexHash))
}

// CheckPayloadCompleteness calculates the hydration ratio of a payload and
// persists it on the session state for cross-process dashboard consumption.
// Emits a telemetry warning if the payload is poorly populated.
func CheckPayloadCompleteness(session *db.SessionState, step string, populated, total int) {
	if total == 0 {
		return
	}
	ratio := float64(populated) / float64(total)
	slog.Info("Payload completeness evaluated", "step", step, "ratio", ratio, "populated", populated, "total", total)

	// Persist on session for BuntDB-backed dashboard access
	if session != nil && session.StepHydrations != nil {
		session.StepHydrations[step] = ratio
	}

	telemetry.GlobalRing.Push(fmt.Sprintf(`{"msg":"Payload completeness evaluated","step":%q,"ratio":%f,"populated":%d,"total":%d}`, step, ratio, populated, total))

	if ratio < 0.8 {
		slog.Warn("TELEMETRY WARNING: Sub-optimal payload hydration detected",
			"step", step,
			"ratio", ratio,
			"populated", populated,
			"total", total,
		)
		telemetry.GlobalRing.Push(fmt.Sprintf(`{"msg":"TELEMETRY WARNING: Sub-optimal payload hydration detected","step":%q,"ratio":%f,"populated":%d,"total":%d}`, step, ratio, populated, total))
	}
}
