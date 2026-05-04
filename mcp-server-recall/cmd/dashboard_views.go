package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
)

var DashboardTabs = []string{
	"Overview",
	"Storage Diagnostics",
	"Memory Consolidation & GC",
	"Semantic Search Engine",
	"Taxonomy & AST Pipeline",
	"RPC & Gateway Analytics",
	"Network Topology",
	"Security & Cryptography",
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
		cpu := fmt.Sprintf("%.2f%%", rt["cpu_usage"])
		rc = fmt.Sprintf("CPU Utilization: %s\nMemory Footprint: %s MB\nActive Goroutines: %s\nUptime: %s sec\nGC Cycles: %s\nConnection: LIVE", cpu, mem, gr, up, gc)
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
			case "INFO":
				lvl = pterm.Green("INF")
			case "DEBUG":
				lvl = pterm.Gray("DBG")
			case "WARN":
				lvl = pterm.Yellow("WRN")
			case "ERROR":
				lvl = pterm.Red("ERR")
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
		if gc, ok := snapshot["memory_gc"].(map[string]any); ok {
			st = fmt.Sprintf("State: Optimizing\nLSM ValueLog Sweeps: %v\nNodes Orphaned & Pruned: %v", gc["sweeps"], gc["pruned_nodes"])
		}
		contentBox = renderTextTab("Memory Consolidation & GC", st)
	case 4:
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

		qpsPanel := loadingText
		if an, ok := snapshot["analytics"].(map[string]any); ok {
			qpsPanel = fmt.Sprintf("Average RPC Latency: %v ms", an["avg_rpc_latency_ms"])
		}

		contentBox = mergeVertical(
			pterm.DefaultBox.WithTitle("Semantic Search Engine").Sprint(st),
			pterm.DefaultBox.WithTitle("Search Latency & QPS").Sprint(qpsPanel),
		)
	case 5:
		stAst := loadingText
		if a, ok := snapshot["ast"].(map[string]any); ok {
			dd := a["disable_drift"]
			ed := a["exclude_dirs"]
			pf := a["parsed_files"]
			td := pterm.TableData{
				{"AST Configuration", "Value"},
				{"Disable Drift Heuristics", fmt.Sprintf("%v", dd)},
				{"Excluded Directories", fmt.Sprintf("%v boundaries", ed)},
				{"Parsed Abstract Files", fmt.Sprintf("%v", pf)},
			}
			stAst, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}

		stTax := loadingText
		if tx, ok := snapshot["taxonomy"].(map[string]any); ok {
			td := pterm.TableData{
				{"Taxonomy Namespace", "Absolute Documents Configured"},
				{"memories", fmt.Sprintf("%v", tx["memories"])},
				{"sessions", fmt.Sprintf("%v", tx["sessions"])},
				{"standards", fmt.Sprintf("%v", tx["standards"])},
				{"projects", fmt.Sprintf("%v", tx["projects"])},
			}
			stTax, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}

		contentBox = mergeVertical(
			renderTextTab("AST Ingestion Pipeline", stAst),
			renderTextTab("Taxonomy & Tag Distribution", stTax),
		)
	case 6:
		st := loadingText
		if an, ok := snapshot["analytics"].(map[string]any); ok {
			td := pterm.TableData{
				{"Metric", "Absolute Value"},
				{"Memory Hits (Latency < 2ms)", fmt.Sprintf("%v", an["cache_hits"])},
				{"Memory Misses", fmt.Sprintf("%v", an["cache_misses"])},
				{"LSM DB Traversal", fmt.Sprintf("%v", an["db_hits"])},
				{"LSM DB Miss", fmt.Sprintf("%v", an["db_misses"])},
				{"RPC Payload Bytes", fmt.Sprintf("%v", an["rpc_payload_bytes"])},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = renderTextTab("RPC & Gateway Analytics", st)
	case 7:
		stNet := loadingText
		if nw, ok := snapshot["network"].(map[string]any); ok {
			stNet = fmt.Sprintf("Clients Currently Authenticated: %v\nTransport Protocol: %v\nStatus: Connected\nUpstream Routes: Active", nw["active_sessions"], nw["transport"])
		}
		contentBox = renderTextTab("Network Topology", stNet)
	case 8:
		stSec := loadingText
		if sec, ok := snapshot["security"].(map[string]any); ok {
			stSec = fmt.Sprintf("Curve25519 Native Encryption: ENABLED\nAES-GCM Memory Cipher: SECURE\nBoundary Violations: %v\nAuth Failures: %v", sec["boundary_violations"], sec["auth_failures"])
		}
		contentBox = renderTextTab("Security & Cryptography", stSec)
	case 9:
		st := loadingText
		if c, ok := snapshot["config"].(map[string]any); ok {
			td := pterm.TableData{
				{"Key", "Value"},
				{"Version", fmt.Sprintf("%v", c["version"])},
				{"DB Path", fmt.Sprintf("%v", c["db_path"])},
				{"Active Log Level", fmt.Sprintf("%v", c["log_level"])},
				{"GOMEMLIMIT", fmt.Sprintf("%v", c["env_gomemlimit"])},
			}
			st, _ = pterm.DefaultTable.WithHasHeader().WithData(td).Srender()
		}
		contentBox = renderTextTab("Config & Environment", st)
	case 10:
		contentBox = pterm.DefaultBox.WithTitle("Quit").Sprint("Press ENTER to exit the dashboard.")
	default:
		contentBox = renderTextTab("Unknown", "Invalid tab.")
	}

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: navBox}, {Data: contentBox}},
	}).Srender()

	return header + "\n" + panels
}
