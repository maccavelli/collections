package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type TelemetryLog struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Pkg   string `json:"pkg"`
	Tool  string `json:"tool"`
}

func ReadDashboardSnapshot() (map[string]any, []TelemetryLog, error) {
	path := filepath.Join(Cfg.GetDBPath(), "telemetry.ring")
	b, err := os.ReadFile(path)
	if err != nil {
		// Just return empty maps if no DB or telemetry file exists yet to avoid crashing the view
		return make(map[string]any), []TelemetryLog{{Msg: "Awaiting daemon telemetry sync..."}}, nil
	}
	
	// Normally we would parse binary fragments from the ring buffer
	var snapshot map[string]any
	
	var logs []TelemetryLog
	lines := strings.Split(string(b), "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if firstLine != "" && strings.HasPrefix(firstLine, "{") {
			json.Unmarshal([]byte(firstLine), &snapshot)
		}
		
		for i := 1; i < len(lines); i++ {
			l := strings.TrimSpace(lines[i])
			if l != "" {
				var tl TelemetryLog
				if err := json.Unmarshal([]byte(l), &tl); err == nil {
					logs = append(logs, tl)
				}
			}
		}
	}
	
	if snapshot == nil {
		snapshot = make(map[string]any)
	}
	return snapshot, logs, nil
}
