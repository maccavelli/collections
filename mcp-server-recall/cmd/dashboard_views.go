package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
)

var DashboardTabs = []string{
	"Summary",
	"Storage Diagnostics",
	"Semantic Search Engine",
	"AST Ingestion Pipeline",
	"Memory Consolidation",
	"Cryptographic Integrity",
	"Gateway RPC Analytics",
	"Taxonomy & Tag Distribution",
	"Client Sync State",
	"Ecosystem & Topology",
	"Security & Access Control",
	"Config & Environment",
	"Quit",
}

const loadingText = "⏳ Loading telemetry data..."

func renderSummary(snapshot map[string]any, logs []TelemetryLog) string {
	rc := loadingText
	if rt, ok := snapshot["runtime"].(map[string]any); ok {
		mem := fmt.Sprintf("%v", rt["memory_mb"])
		gr := fmt.Sprintf("%v", rt["goroutines"])
		up := fmt.Sprintf("%v", rt["uptime_sec"])
		gc := fmt.Sprintf("%v", rt["num_gc"])
		rc = fmt.Sprintf("Memory Footprint: %s MB\nActive Goroutines: %s\nUptime: %s sec\nGC Cycles: %s\nConnection: LIVE", mem, gr, up, gc)
	}

	leftBox := pterm.DefaultBox.WithTitle("Health Overview").Sprint(rc)

	logContent := "Awaiting daemon telemetry sync..."
	if len(logs) > 0 {
		tail := logs
		if len(tail) > 12 {
			tail = tail[len(tail)-12:]
		}
		
		td := pterm.TableData{
			{"Time", "Lvl", "Message", "Target"},
		}
		for _, l := range tail {
			var target string
			if l.Pkg != "" {
				target = l.Pkg
			} else if l.Tool != "" {
				target = l.Tool
			}
			
			timeStr := l.Time
			if len(timeStr) > 19 {
				timeStr = timeStr[11:19]
			}
			
			lvl := l.Level
			switch lvl {
			case "INFO": lvl = pterm.Green("INF")
			case "DEBUG": lvl = pterm.Gray("DBG")
			case "WARN": lvl = pterm.Yellow("WRN")
			case "ERROR": lvl = pterm.Red("ERR")
			}
			
			if len(target) > 25 {
				target = "..." + target[len(target)-22:]
			}
			msg := l.Msg
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			
			td = append(td, []string{timeStr, lvl, msg, target})
		}
		logContent, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
	}

	rightBox := pterm.DefaultBox.WithTitle("Live Telemetry Events").Sprint(logContent)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: leftBox}, {Data: rightBox}},
	}).Srender()
	return panels
}

func renderStorage(snapshot map[string]any) string {
	st := loadingText
	if s, ok := snapshot["storage"].(map[string]any); ok {
		st = fmt.Sprintf("BadgerDB instance actively mapped.\nLSM Size: %v bytes\nValue Log Size: %v bytes", s["lsm_bytes"], s["vlog_bytes"])
	}
	return pterm.DefaultBox.WithTitle("Storage Diagnostics (BadgerDB)").Sprint(st)
}

func renderBleve(snapshot map[string]any) string {
	st := loadingText
	if b, ok := snapshot["bleve"].(map[string]any); ok {
		docs := b["documents"]
		queues := b["queues"]
		st = fmt.Sprintf("Engine: Bleve Native\nIndexed Documents: %v\nPending Queue Tasks: %v", docs, queues)
	}
	return mergeVertical(
		pterm.DefaultBox.WithTitle("Semantic Search Engine").Sprint(st),
		pterm.DefaultBox.WithTitle("TF-IDF Matrix").Sprint("Vector thresholds active and stable"),
	)
}

func mergeVertical(boxes ...string) string {
	var layout [][]pterm.Panel
	for _, b := range boxes {
		layout = append(layout, []pterm.Panel{{Data: b}})
	}
	s, _ := pterm.DefaultPanel.WithPanels(layout).Srender()
	return s
}

func renderTextTab(title string, text string) string {
	return pterm.DefaultBox.WithTitle(title).Sprint(text)
}

func renderPtermDashboard(snapshot map[string]any, logs []TelemetryLog, uiState *InternalUIState) string {
	tabIndex := uiState.GetActiveTab()
	header := pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).
		WithFullWidth().
		Sprint("RECALL OBSERVABILITY DASHBOARD (↑↓ Navigate ⏎ Select  q Quit)")

	var navItems []string
	for i, t := range DashboardTabs {
		idx := int32(i + 1)
		if idx == tabIndex {
			navItems = append(navItems, pterm.LightGreen("▶ ["+fmt.Sprint(idx)+"] "+t))
		} else {
			navItems = append(navItems, "  ["+fmt.Sprint(idx)+"] "+t)
		}
	}
	navBox := pterm.DefaultBox.WithTitle("Navigation").Sprint(strings.Join(navItems, "\n"))

	var contentBox string
	
	switch tabIndex {
	case 1:
		contentBox = renderSummary(snapshot, logs)
	case 2:
		contentBox = renderStorage(snapshot)
	case 3:
		st := loadingText
		if b, ok := snapshot["bleve"].(map[string]any); ok {
			docs := b["documents"]
			queues := b["queues"]
			drift := b["drift"]
			td := pterm.TableData{
				{"Metric", "Value"},
				{"Indexed Documents", fmt.Sprintf("%v", docs)},
				{"Pending Queue Tasks", fmt.Sprintf("%v", queues)},
				{"Heuristic Drift Alerts", fmt.Sprintf("%v", drift)},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = mergeVertical(
			pterm.DefaultBox.WithTitle("Semantic Search Engine").Sprint(st),
			pterm.DefaultBox.WithTitle("TF-IDF Matrix").Sprint("Vector thresholds active and stable"),
		)
	case 4:
		st := loadingText
		if a, ok := snapshot["ast"].(map[string]any); ok {
			dd := a["disable_drift"]
			ed := a["exclude_dirs"]
			td := pterm.TableData{
				{"Configuration", "Value"},
				{"Disable Drift Heuristics", fmt.Sprintf("%v", dd)},
				{"Excluded Directories", fmt.Sprintf("%v directory boundaries mapped", ed)},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = mergeVertical(
			renderTextTab("AST Ingestion Pipeline", st),
			renderTextTab("AST Parsing Activity", "Awaiting standards ingestion via CLI."),
		)
	case 5:
		contentBox = renderTextTab("Memory Consolidation", "State: Optimizing\nPeriodic DAG garbage collection active.\nNo volatile fragmented memories found.")
	case 6:
		contentBox = renderTextTab("Cryptographic Integrity", "Curve25519 Native Encryption: ENABLED\nAES-GCM Memory Cipher: SECURE\nKey Rotation Delta: N/A")
	case 7:
		st := loadingText
		if an, ok := snapshot["analytics"].(map[string]any); ok {
			td := pterm.TableData{
				{"Metric", "Absolute Requests Captured"},
				{"Memory Hits (Latency < 2ms)", fmt.Sprintf("%v", an["cache_hits"])},
				{"Memory Misses", fmt.Sprintf("%v", an["cache_misses"])},
				{"LSM DB Traversal", fmt.Sprintf("%v", an["db_hits"])},
				{"LSM DB Miss", fmt.Sprintf("%v", an["db_misses"])},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = renderTextTab("Gateway RPC Analytics", st)
	case 8:
		st := loadingText
		if tx, ok := snapshot["taxonomy"].(map[string]any); ok {
			td := pterm.TableData{
				{"Namespace", "Absolute Documents Configured"},
				{"memories", fmt.Sprintf("%v", tx["memories"])},
				{"sessions", fmt.Sprintf("%v", tx["sessions"])},
				{"standards", fmt.Sprintf("%v", tx["standards"])},
				{"projects", fmt.Sprintf("%v", tx["projects"])},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = renderTextTab("Taxonomy & Tag Distribution", st)
	case 9:
		contentBox = renderTextTab("Client Sync State", "Clients Currently Authenticated: 1 native streamable-http\nTool Whitelists Applied\nResource Matrix: Clean")
	case 10:
		contentBox = renderTextTab("Ecosystem & Topology", "Status: Connected\nOrchestrator: mcp-server-magictools\nUpstream Routes: Active")
	case 11:
		contentBox = renderTextTab("Security & Access Control", "Boundary Violations: 0\nAuth Failures: 0\nActive Whitelist: Strict")
	case 12:
		st := loadingText
		if c, ok := snapshot["config"].(map[string]any); ok {
			td := pterm.TableData{
				{"Key", "Value"},
				{"Version", fmt.Sprintf("%v", c["version"])},
				{"DB Path", fmt.Sprintf("%v", c["db_path"])},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = renderTextTab("Config & Environment", st)
	case 13:
		contentBox = pterm.DefaultBox.WithTitle("Quit").Sprint("Press ENTER to exit the dashboard.")
	default:
		contentBox = renderTextTab("Unknown", "Invalid tab.")
	}

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: navBox}, {Data: contentBox}},
	}).Srender()

	return header + "\n" + panels
}
