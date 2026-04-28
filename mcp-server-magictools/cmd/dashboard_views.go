package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const loadingText = "⏳ Loading telemetry data..."

// DashboardTabs natively defines the active array of menu items, allowing dashboard.go to dynamically scale keyboard constraints avoiding boundary panics explicitly.
var DashboardTabs = []string{"Overview", "Tool-Registry", "Intelligence", "Orchestration", "Storage", "Gateway", "Diagnostics", "Quit"}

func mergeVertical(boxes ...string) string {
	var layout [][]pterm.Panel
	for _, b := range boxes {
		layout = append(layout, []pterm.Panel{{Data: b}})
	}
	s, _ := pterm.DefaultPanel.WithPanels(layout).Srender()
	return s
}

func renderPtermDashboard(snapshot map[string]any, logs []string, uiState *InternalUIState) string {
	tabIndex := uiState.GetActiveTab()
	header := pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).
		WithFullWidth().
		Sprint("MAGICTOOLS OBSERVABILITY DASHBOARD (↑↓ Navigate ⏎ Select  q Quit)")

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
		contentBox = mergeVertical(renderOverview(snapshot, logs), renderFleet(snapshot))
	case 2:
		contentBox = mergeVertical(renderTools(snapshot), renderScores(snapshot))
	case 3:
		contentBox = mergeVertical(renderSearch(snapshot), renderRAG(snapshot))
	case 4:
		contentBox = mergeVertical(renderPipeline(snapshot), renderSpans(logs, uiState))
	case 5:
		contentBox = mergeVertical(renderDatabases(snapshot), renderCollisions(snapshot))
	case 6:
		contentBox = mergeVertical(renderProxy(snapshot), renderTokenValue(snapshot), renderComms(snapshot))
	case 7:
		contentBox = mergeVertical(renderErrors(snapshot), renderRuntime(snapshot))
	case 8:
		contentBox = pterm.DefaultBox.WithTitle("Quit").Sprint("Press ENTER to exit the dashboard.")
	default:
		contentBox = mergeVertical(renderOverview(snapshot, logs), renderFleet(snapshot))
	}

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: navBox}, {Data: contentBox}},
	}).Srender()

	return header + "\n" + panels
}

// ── Tab 1: Overview ─────────────────────────────────────────────────────────

func renderOverview(snapshot map[string]any, logs []string) string {
	serverDetails := loadingText
	serversRaw, ok := snapshot["servers"].([]any)
	if ok && len(serversRaw) > 0 {
		var rows [][]string
		rows = append(rows, []string{"Server", "Status", "Uptime", "Ping"})
		for _, raw := range serversRaw {
			if s, ok := raw.(map[string]any); ok {
				name := str(s, "name")
				uptime := str(s, "uptime")
				ping := str(s, "ping_latency")
				statusStr := pterm.Red("OFF")
				if s["running"] == true {
					statusStr = pterm.Green("ON")
				}
				rows = append(rows, []string{name, statusStr, uptime, ping})
			}
		}
		if len(rows) > 1 {
			tables, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
			serverDetails = tables
		}
	}

	leftBox := pterm.DefaultBox.WithTitle("Overview").Sprint(serverDetails)

	logsStr := loadingText
	if len(logs) > 0 {
		tail := logs
		if len(tail) > 10 {
			tail = tail[len(tail)-10:]
		}
		var items []pterm.BulletListItem
		for _, l := range tail {
			if len(l) > 100 {
				l = l[:100] + "..."
			}
			items = append(items, pterm.BulletListItem{Level: 0, Text: l})
		}
		logsStr, _ = pterm.DefaultBulletList.WithItems(items).Srender()
	}

	rightBox := pterm.DefaultBox.WithTitle("Recent Events").Sprint(logsStr)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: leftBox}, {Data: rightBox}},
	}).Srender()
	return panels
}

// ── Tab 2: Fleet ────────────────────────────────────────────────────────────

func renderFleet(snapshot map[string]any) string {
	serversRaw, ok := snapshot["servers"].([]any)
	if !ok || len(serversRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Fleet Status").Sprint(loadingText)
	}

	rows := [][]string{
		{"Server", "Status", "Uptime", "Calls", "Latency", "Ping", "Errors", "RSS", "CPU"},
	}
	for _, raw := range serversRaw {
		s, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		statusStr := pterm.Red("●")
		if s["running"] == true {
			statusStr = pterm.Green("●")
		}
		rows = append(rows, []string{
			str(s, "name"),
			statusStr,
			str(s, "uptime"),
			str(s, "total_calls"),
			str(s, "last_latency"),
			str(s, "ping_latency"),
			str(s, "consecutive_errors"),
			str(s, "memory_rss"),
			str(s, "cpu_usage"),
		})
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	return pterm.DefaultBox.WithTitle("Fleet Status").Sprint(table)
}

// ── Tab 3: Scores ───────────────────────────────────────────────────────────

func renderScores(snapshot map[string]any) string {
	rawScores, ok := snapshot["scores"].(map[string]ToolScoreCard)
	if !ok || len(rawScores) == 0 {
		return pterm.DefaultBox.WithTitle("Tool Reliability Scores").Sprint("Waiting for tool calls... scores appear after the first proxy call.")
	}

	// Sort by reliability descending
	var arr []ToolScoreCard
	for _, card := range rawScores {
		arr = append(arr, card)
	}
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].Reliability > arr[j].Reliability
	})
	if len(arr) > 20 {
		arr = arr[:20]
	}

	rows := [][]string{
		{"URN", "Rel%", "Base", "Dev", "30m Δ", "4h Δ", "All Δ"},
	}

	for _, c := range arr {
		// Reliability percentage (color coded)
		relPct := c.Reliability * 100
		relStr := fmt.Sprintf("%.1f%%", relPct)
		if relPct >= 95 {
			relStr = pterm.Green(relStr)
		} else if relPct >= 80 {
			relStr = pterm.Yellow(relStr)
		} else {
			relStr = pterm.Red(relStr)
		}

		rows = append(rows, []string{
			c.URN,
			relStr,
			fmt.Sprintf("%.3f", c.Baseline),
			colorDelta(c.Deviation),
			colorDelta(c.Delta30m),
			colorDelta(c.Delta4h),
			colorDelta(c.DeltaAll),
		})
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	title := fmt.Sprintf("Tool Reliability (Top %d) — 10s refresh", len(arr))
	return pterm.DefaultBox.WithTitle(title).Sprint(table)
}

// colorDelta formats a float delta with +/- sign and green/red/neutral coloring.
func colorDelta(d float64) string {
	s := fmt.Sprintf("%+.3f", d)
	if d > 0.001 {
		return pterm.Green(s)
	} else if d < -0.001 {
		return pterm.Red(s)
	}
	return s
}

// ── Tab 4: Databases ────────────────────────────────────────────────────────

func getHist(dbsHist map[string]any, window string, dbName string) map[string]any {
	if dbsHist == nil {
		return nil
	}
	winObj, _ := dbsHist[window].(map[string]any)
	if winObj == nil {
		return nil
	}
	dbObj, _ := winObj[dbName].(map[string]any)
	return dbObj
}

func dbDeltaStr(live map[string]any, hist map[string]any, key string) string {
	liveVal := numF64(live, key)
	if hist == nil {
		return "-"
	}
	// Missing keys default to 0, matching numF64 behavior safely.
	histVal := numF64(hist, key)
	diff := liveVal - histVal
	if diff > 0 {
		return pterm.Green(fmt.Sprintf("+%v", diff))
	} else if diff < 0 {
		return pterm.Red(fmt.Sprintf("%v", diff))
	}
	return "0"
}

func renderDatabases(snapshot map[string]any) string {
	dbsRaw, ok := snapshot["databases"].(map[string]any)
	if !ok || len(dbsRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Database Diagnostics").Sprint("Waiting for database metrics telemetry...")
	}
	dbsHist, _ := snapshot["databases_history"].(map[string]any)

	magicRaw, magicOk := dbsRaw["magictools"].(map[string]any)
	recallRaw, recallOk := dbsRaw["recall"].(map[string]any)

	var magicBox, recallBox string

	// MagicTools DB Box
	if magicOk {
		m5 := getHist(dbsHist, "5m", "magictools")
		m15 := getHist(dbsHist, "15m", "magictools")
		m60 := getHist(dbsHist, "1h", "magictools")

		rows := [][]string{
			{"Metric", "Live", "5m Δ", "15m Δ", "1h Δ"},
			{"Registry Cache Hits", fmt.Sprint(numF64(magicRaw, "Hits")), dbDeltaStr(magicRaw, m5, "Hits"), dbDeltaStr(magicRaw, m15, "Hits"), dbDeltaStr(magicRaw, m60, "Hits")},
			{"Registry Cache Misses", fmt.Sprint(numF64(magicRaw, "Misses")), dbDeltaStr(magicRaw, m5, "Misses"), dbDeltaStr(magicRaw, m15, "Misses"), dbDeltaStr(magicRaw, m60, "Misses")},
			{"Registry Cached Items", fmt.Sprint(numF64(magicRaw, "Entries")), dbDeltaStr(magicRaw, m5, "Entries"), dbDeltaStr(magicRaw, m15, "Entries"), dbDeltaStr(magicRaw, m60, "Entries")},
			{"BadgerDB Tools Count", fmt.Sprint(numF64(magicRaw, "Tools")), dbDeltaStr(magicRaw, m5, "Tools"), dbDeltaStr(magicRaw, m15, "Tools"), dbDeltaStr(magicRaw, m60, "Tools")},
			{"BadgerDB Intel Count", fmt.Sprint(numF64(magicRaw, "Intel")), dbDeltaStr(magicRaw, m5, "Intel"), dbDeltaStr(magicRaw, m15, "Intel"), dbDeltaStr(magicRaw, m60, "Intel")},
			{"Bleve Search Docs", fmt.Sprint(numF64(magicRaw, "BleveDocs")), dbDeltaStr(magicRaw, m5, "BleveDocs"), dbDeltaStr(magicRaw, m15, "BleveDocs"), dbDeltaStr(magicRaw, m60, "BleveDocs")},
		}
		table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
		magicBox = pterm.DefaultBox.WithTitle("Orchestrator DB (MagicTools) 10s refresh").Sprint(table)
	} else {
		magicBox = pterm.DefaultBox.WithTitle("Orchestrator DB (MagicTools)").Sprint("Offline")
	}

	// Recall DB Box
	if recallOk {
		r5 := getHist(dbsHist, "5m", "recall")
		r15 := getHist(dbsHist, "15m", "recall")
		r60 := getHist(dbsHist, "1h", "recall")

		rows := [][]string{
			{"Metric", "Live", "5m Δ", "15m Δ", "1h Δ"},
			{"Memories Count", fmt.Sprint(numF64(recallRaw, "Memories")), dbDeltaStr(recallRaw, r5, "Memories"), dbDeltaStr(recallRaw, r15, "Memories"), dbDeltaStr(recallRaw, r60, "Memories")},
			{"Sessions Count", fmt.Sprint(numF64(recallRaw, "Sessions")), dbDeltaStr(recallRaw, r5, "Sessions"), dbDeltaStr(recallRaw, r15, "Sessions"), dbDeltaStr(recallRaw, r60, "Sessions")},
			{"Standards Count", fmt.Sprint(numF64(recallRaw, "Standards")), dbDeltaStr(recallRaw, r5, "Standards"), dbDeltaStr(recallRaw, r15, "Standards"), dbDeltaStr(recallRaw, r60, "Standards")},
			{"Vector Cache Hits", fmt.Sprint(numF64(recallRaw, "Hits")), dbDeltaStr(recallRaw, r5, "Hits"), dbDeltaStr(recallRaw, r15, "Hits"), dbDeltaStr(recallRaw, r60, "Hits")},
			{"Vector Cache Misses", fmt.Sprint(numF64(recallRaw, "Misses")), dbDeltaStr(recallRaw, r5, "Misses"), dbDeltaStr(recallRaw, r15, "Misses"), dbDeltaStr(recallRaw, r60, "Misses")},
			{"Vector Search Docs", fmt.Sprint(numF64(recallRaw, "BleveDocs")), dbDeltaStr(recallRaw, r5, "BleveDocs"), dbDeltaStr(recallRaw, r15, "BleveDocs"), dbDeltaStr(recallRaw, r60, "BleveDocs")},
		}
		table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
		recallBox = pterm.DefaultBox.WithTitle("Memory DB (Recall) 10s refresh").Sprint(table)
	} else {
		recallBox = pterm.DefaultBox.WithTitle("Memory DB (Recall)").Sprint("Offline or Disconnected")
	}

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: magicBox}, {Data: recallBox}},
	}).Srender()

	return panels
}

// ── Tab 4: Tools ────────────────────────────────────────────────────────────

func renderTools(snapshot map[string]any) string {
	toolsRaw, ok := snapshot["tools"].(map[string]any)
	if !ok || len(toolsRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Tool Analytics").Sprint("No tool telemetry data recorded yet.")
	}

	// Sort URNs for stable rendering
	var urns []string
	for u := range toolsRaw {
		urns = append(urns, u)
	}
	sort.Strings(urns)

	rows := [][]string{
		{"URN", "Calls", "Avg ms", "Faults", "Last Call"},
	}
	for _, urn := range urns {
		raw := toolsRaw[urn]
		t, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		calls := numI64(t, "Calls")
		totalMs := numI64(t, "TotalMs")
		faults := numI64(t, "Faults")
		lastAt := numI64(t, "LastCallAt")

		avgMs := int64(0)
		if calls > 0 {
			avgMs = totalMs / calls
		}

		// Colour-code latency
		latStr := fmt.Sprintf("%d", avgMs)
		if avgMs > 500 {
			latStr = pterm.Red(latStr)
		} else if avgMs > 100 {
			latStr = pterm.Yellow(latStr)
		} else {
			latStr = pterm.Green(latStr)
		}

		lastCallStr := "-"
		if lastAt > 0 {
			lastCallStr = time.Since(time.Unix(0, lastAt)).Truncate(time.Second).String() + " ago"
		}

		rows = append(rows, []string{
			urn,
			fmt.Sprint(calls),
			latStr,
			fmt.Sprint(faults),
			lastCallStr,
		})
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	return pterm.DefaultBox.WithTitle("Tool Analytics").Sprint(table)
}

// ── Tab 4: Pipeline ─────────────────────────────────────────────────────────

func renderPipeline(snapshot map[string]any) string {
	optRaw, ok := snapshot["opt_metrics"].(map[string]any)
	if !ok || len(optRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Pipeline Optimization").Sprint(loadingText)
	}

	squeezeRows := [][]string{
		{"Metric", "Value"},
		{"Bypass Count", str(optRaw, "squeeze_bypass")},
		{"Truncations", str(optRaw, "squeeze_trunc")},
	}
	squeezeTable, _ := pterm.DefaultTable.WithHasHeader().WithData(squeezeRows).Srender()
	squeezeBox := pterm.DefaultBox.WithTitle("Squeeze Writer").Sprint(squeezeTable)

	hfscRows := [][]string{
		{"Metric", "Value"},
		{"Reassembly OK", str(optRaw, "hfsc_success")},
		{"Reassembly Fail", str(optRaw, "hfsc_fail")},
		{"Swept Stale", str(optRaw, "hfsc_swept")},
		{"Active Streams", str(optRaw, "hfsc_active")},
	}
	hfscTable, _ := pterm.DefaultTable.WithHasHeader().WithData(hfscRows).Srender()
	hfscBox := pterm.DefaultBox.WithTitle("HFSC Fragmenter").Sprint(hfscTable)

	cssaRows := [][]string{
		{"Metric", "Value"},
		{"Offload Bytes", str(optRaw, "cssa_offload")},
		{"Sync Operations", str(optRaw, "cssa_sync")},
	}
	cssaTable, _ := pterm.DefaultTable.WithHasHeader().WithData(cssaRows).Srender()
	cssaBox := pterm.DefaultBox.WithTitle("CSSA Offload").Sprint(cssaTable)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: squeezeBox}, {Data: hfscBox}, {Data: cssaBox}},
	}).Srender()

	return panels
}

// ── Tab 5: Errors ───────────────────────────────────────────────────────────

func renderErrors(snapshot map[string]any) string {
	errRaw, errOk := snapshot["errors"].(map[string]any)
	lcRaw, lcOk := snapshot["lifecycle"].(map[string]any)

	if !errOk && !lcOk {
		return pterm.DefaultBox.WithTitle("Errors & Alerts").Sprint(loadingText)
	}

	// Error taxonomy table
	errContent := loadingText
	if errOk {
		errRows := [][]string{
			{"Category", "Count"},
			{"Timeout", str(errRaw, "timeout")},
			{"Connection Refused", str(errRaw, "connection_refused")},
			{"Panic", str(errRaw, "panic")},
			{"Validation", str(errRaw, "validation")},
			{"Hallucination", str(errRaw, "hallucination")},
			{"Pipe Error", str(errRaw, "pipe_error")},
			{"Context Cancelled", str(errRaw, "context_cancelled")},
		}
		errTable, _ := pterm.DefaultTable.WithHasHeader().WithData(errRows).Srender()
		errContent = errTable
	}
	errBox := pterm.DefaultBox.WithTitle("Error Taxonomy").Sprint(errContent)

	// Lifecycle table
	lcContent := loadingText
	if lcOk {
		lcRows := [][]string{
			{"Event", "Count"},
			{"Restarts (Health)", str(lcRaw, "restarts_health")},
			{"Restarts (OOM)", str(lcRaw, "restarts_oom")},
			{"Evictions", str(lcRaw, "evictions")},
			{"Reconnections", str(lcRaw, "reconnections")},
			{"Config Reloads", str(lcRaw, "config_reloads")},
			{"Backpressure Pending", str(lcRaw, "backpressure_pending")},
			{"Backpressure Rejected", str(lcRaw, "backpressure_reject")},
		}
		lcTable, _ := pterm.DefaultTable.WithHasHeader().WithData(lcRows).Srender()
		lcContent = lcTable
	}
	lcBox := pterm.DefaultBox.WithTitle("Lifecycle Events").Sprint(lcContent)

	// Recent errors ring — parse and word-wrap to terminal width
	recentContent := "No recent errors."
	if reRaw, ok := snapshot["recent_errors"].([]any); ok && len(reRaw) > 0 {
		termWidth := pterm.GetTerminalWidth()
		if termWidth < 40 {
			termWidth = 80
		}
		// Reserve space for box borders and bullet indent
		wrapWidth := termWidth - 10

		var lines []string
		for _, e := range reRaw {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			server, _ := entry["Server"].(string)
			corrID, _ := entry["CorrelationID"].(string)
			msg, _ := entry["Message"].(string)
			if msg == "" {
				msg = fmt.Sprintf("%v", e)
			}

			// Compact header: [server] corr_id_prefix
			header := fmt.Sprintf("[%s]", server)
			if len(corrID) > 8 {
				corrID = corrID[:8]
			}
			if corrID != "" {
				header += " " + corrID
			}

			// Word-wrap the message
			wrapped := wordWrap(msg, wrapWidth-2)
			lines = append(lines, pterm.Red("● ")+pterm.Bold.Sprint(header))
			for _, wl := range strings.Split(wrapped, "\n") {
				lines = append(lines, "  "+pterm.Red(wl))
			}
			lines = append(lines, "")
		}
		recentContent = strings.Join(lines, "\n")
	}
	recentBox := pterm.DefaultBox.WithTitle("Recent Errors").Sprint(recentContent)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: errBox}, {Data: lcBox}},
		{{Data: recentBox}},
	}).Srender()

	return panels
}

// ── Tab 6: Events ───────────────────────────────────────────────────────────

func renderEvents(logs []string) string {
	if len(logs) == 0 {
		return pterm.DefaultBox.WithTitle("Live Events").Sprint(loadingText)
	}

	// Show last 25 entries for density
	tail := logs
	if len(tail) > 25 {
		tail = tail[len(tail)-25:]
	}

	var items []pterm.BulletListItem
	for _, l := range tail {
		if len(l) > 120 {
			l = l[:120] + "..."
		}
		items = append(items, pterm.BulletListItem{Level: 0, Text: l})
	}

	list, _ := pterm.DefaultBulletList.WithItems(items).Srender()
	return pterm.DefaultBox.WithTitle("Live Events (last 25)").Sprint(list)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func str(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return "-"
	}
	return fmt.Sprintf("%v", v)
}

func numF64(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

func numI64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// wordWrap breaks text into lines no longer than maxWidth characters,
// splitting at word boundaries.
func wordWrap(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}



// ── Tab 12: Proxy ───────────────────────────────────────────────────────────

func renderProxy(snapshot map[string]any) string {
	proxyRaw, ok := snapshot["proxy"].(map[string]any)
	if !ok || len(proxyRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Proxy Telemetry").Sprint("Waiting for proxy call data...")
	}

	// Per-server throughput table
	serverContent := "No calls recorded yet."
	if serversRaw, ok := proxyRaw["servers"].(map[string]any); ok && len(serversRaw) > 0 {
		rows := [][]string{
			{"Server", "Calls", "Spinup ms", "Bytes Sent", "Bytes Raw", "Bytes Min", "Faults", "Soft Fail"},
		}
		var names []string
		for n := range serversRaw {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			raw := serversRaw[name]
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, []string{
				name,
				fmt.Sprint(numI64(m, "calls")),
				fmt.Sprint(numI64(m, "total_spinup_ms")),
				fmt.Sprint(numI64(m, "bytes_sent")),
				fmt.Sprint(numI64(m, "bytes_raw")),
				fmt.Sprint(numI64(m, "bytes_minified")),
				fmt.Sprint(numI64(m, "faults")),
				fmt.Sprint(numI64(m, "soft_failures")),
			})
		}
		table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
		serverContent = table
	}
	serverBox := pterm.DefaultBox.WithTitle("Per-Server Throughput").Sprint(serverContent)

	// EMA Latencies table
	latContent := "No latency data yet."
	if latRaw, ok := proxyRaw["latencies"].(map[string]any); ok && len(latRaw) > 0 {
		latRows := [][]string{
			{"Pipeline Stage", "EMA (ms)", "Count"},
			{"align_tools", fmt.Sprintf("%.1f", numF64(latRaw, "align_tools_ema")), fmt.Sprint(numI64(latRaw, "align_tools_count"))},
			{"call_proxy", fmt.Sprintf("%.1f", numF64(latRaw, "call_proxy_ema")), fmt.Sprint(numI64(latRaw, "call_proxy_count"))},
			{"call_proxy (hot)", fmt.Sprintf("%.1f", numF64(latRaw, "call_proxy_hot_ema")), fmt.Sprint(numI64(latRaw, "call_proxy_hot_cnt"))},
			{"boot (cold)", fmt.Sprintf("%.1f", numF64(latRaw, "boot_ema")), fmt.Sprint(numI64(latRaw, "boot_count"))},
		}
		latTable, _ := pterm.DefaultTable.WithHasHeader().WithData(latRows).Srender()
		latContent = latTable
	}
	latBox := pterm.DefaultBox.WithTitle("EMA Latencies (10% α)").Sprint(latContent)

	// Session digest
	digestContent := "No session data yet."
	if statsRaw, ok := proxyRaw["session_stats"].(map[string]any); ok {
		if digestRaw, ok := statsRaw["digest"].(map[string]any); ok {
			digestRows := [][]string{
				{"Metric", "Value"},
				{"Total Calls", str(digestRaw, "total_calls")},
				{"Total Faults", str(digestRaw, "total_faults")},
				{"Tokens Used (est)", str(digestRaw, "tokens_used")},
				{"Tokens Saved (est)", str(digestRaw, "tokens_saved")},
			}
			digestTable, _ := pterm.DefaultTable.WithHasHeader().WithData(digestRows).Srender()
			digestContent = digestTable
		}
	}
	digestBox := pterm.DefaultBox.WithTitle("Session Digest").Sprint(digestContent)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: serverBox}},
		{{Data: latBox}, {Data: digestBox}},
	}).Srender()

	return panels
}

// ── Tab 13: Runtime ─────────────────────────────────────────────────────────

func renderRuntime(snapshot map[string]any) string {
	rtRaw, ok := snapshot["runtime"].(map[string]any)
	if !ok || len(rtRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Go Runtime").Sprint("Waiting for runtime metrics...")
	}

	// GOMEMLIMIT headroom
	headroomStr := "N/A (not set)"
	goMemLimitMB := numF64(rtRaw, "go_mem_limit_mb")
	if goMemLimitMB > 0 {
		headroom := numF64(rtRaw, "headroom_pct")
		headroomStr = fmt.Sprintf("%.1f%%", headroom)
		if headroom < 20 {
			headroomStr = pterm.Red(headroomStr)
		} else if headroom < 50 {
			headroomStr = pterm.Yellow(headroomStr)
		} else {
			headroomStr = pterm.Green(headroomStr)
		}
	}

	memRows := [][]string{
		{"Metric", "Value"},
		{"Heap Alloc", fmt.Sprintf("%.1f MB", numF64(rtRaw, "heap_alloc_mb"))},
		{"Heap Sys", fmt.Sprintf("%.1f MB", numF64(rtRaw, "heap_sys_mb"))},
		{"GC Cycles", fmt.Sprint(numI64(rtRaw, "num_gc"))},
		{"GC Pause Total", fmt.Sprintf("%.1f ms", numF64(rtRaw, "pause_total_ms"))},
		{"Goroutines", fmt.Sprint(numI64(rtRaw, "num_goroutine"))},
		{"GOMAXPROCS", fmt.Sprint(numI64(rtRaw, "go_max_procs"))},
	}
	if goMemLimitMB > 0 {
		memRows = append(memRows, []string{"GOMEMLIMIT", fmt.Sprintf("%.0f MB", goMemLimitMB)})
	}
	memRows = append(memRows, []string{"GOMEMLIMIT Headroom", headroomStr})

	memTable, _ := pterm.DefaultTable.WithHasHeader().WithData(memRows).Srender()
	memBox := pterm.DefaultBox.WithTitle("Orchestrator Go Runtime").Sprint(memTable)

	return memBox
}

// ── Tab 14: Search ──────────────────────────────────────────────────────────

func renderSearch(snapshot map[string]any) string {
	searchRaw, ok := snapshot["search"].(map[string]any)
	if !ok || len(searchRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Search Intelligence").Sprint("Waiting for search telemetry...")
	}

	// Mode indicator
	mode := str(searchRaw, "mode")
	modeDisplay := pterm.Yellow("● " + mode)
	if strings.Contains(mode, "Vector") {
		modeDisplay = pterm.Green("● " + mode)
	}

	totalSearches := numI64(searchRaw, "total_searches")
	vectorSearches := numI64(searchRaw, "vector_searches")
	lexicalSearches := numI64(searchRaw, "lexical_searches")

	// Calculate vector ratio
	vectorRatioStr := "N/A"
	if totalSearches > 0 {
		ratio := float64(vectorSearches) / float64(totalSearches) * 100
		vectorRatioStr = fmt.Sprintf("%.1f%%", ratio)
		if ratio >= 80 {
			vectorRatioStr = pterm.Green(vectorRatioStr)
		} else if ratio >= 50 {
			vectorRatioStr = pterm.Yellow(vectorRatioStr)
		}
	}

	rows := [][]string{
		{"Metric", "Value"},
		{"Active Mode", modeDisplay},
		{"Total Searches", fmt.Sprint(totalSearches)},
		{"Vector (HNSW)", fmt.Sprint(vectorSearches)},
		{"Lexical (Bleve)", fmt.Sprint(lexicalSearches)},
		{"Vector Ratio", vectorRatioStr},
		{"Search Latency (total ms)", fmt.Sprint(numI64(searchRaw, "total_latency_ms"))},
		{"Cache Hits", fmt.Sprint(numI64(searchRaw, "cache_hits"))},
		{"Cache Misses", fmt.Sprint(numI64(searchRaw, "cache_misses"))},
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	return pterm.DefaultBox.WithTitle("Search Intelligence").Sprint(table)
}

// ── Tab 15: Comms ───────────────────────────────────────────────────────────

func renderComms(snapshot map[string]any) string {
	proxyRaw, ok := snapshot["proxy"].(map[string]any)
	if !ok || len(proxyRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Comms Transport").Sprint("Waiting for transport data...")
	}

	// Per-server bytes and squeeze ratios
	serverContent := "No transport data yet."
	if serversRaw, ok := proxyRaw["servers"].(map[string]any); ok && len(serversRaw) > 0 {
		rows := [][]string{
			{"Server", "Calls", "Bytes Out", "Bytes Raw", "Bytes Min", "Squeeze %", "Faults", "Health"},
		}
		var names []string
		for n := range serversRaw {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			raw := serversRaw[name]
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			bytesRaw := numI64(m, "bytes_raw")
			bytesMin := numI64(m, "bytes_minified")
			faults := numI64(m, "faults")
			calls := numI64(m, "calls")

			// Squeeze ratio
			squeezeStr := "N/A"
			if bytesRaw > 0 {
				ratio := float64(bytesMin) / float64(bytesRaw) * 100
				squeezeStr = fmt.Sprintf("%.1f%%", ratio)
				if ratio < 50 {
					squeezeStr = pterm.Green(squeezeStr) // high compression
				} else if ratio < 80 {
					squeezeStr = pterm.Yellow(squeezeStr)
				}
			}

			// Health indicator
			health := pterm.Green("● OK")
			if faults > 0 && calls > 0 {
				faultRate := float64(faults) / float64(calls) * 100
				if faultRate > 10 {
					health = pterm.Red("● DEGRADED")
				} else if faultRate > 2 {
					health = pterm.Yellow("● WARN")
				}
			}

			rows = append(rows, []string{
				name,
				fmt.Sprint(calls),
				fmt.Sprint(numI64(m, "bytes_sent")),
				fmt.Sprint(bytesRaw),
				fmt.Sprint(bytesMin),
				squeezeStr,
				fmt.Sprint(faults),
				health,
			})
		}
		table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
		serverContent = table
	}

	return pterm.DefaultBox.WithTitle("Comms Transport (Per-Server I/O)").Sprint(serverContent)
}

// ── Tab 9: Collisions ───────────────────────────────────────────────────────

func renderCollisions(snapshot map[string]any) string {
	raw, ok := snapshot["collisions"].(map[string]any)
	if !ok || len(raw) == 0 {
		return pterm.DefaultBox.WithTitle("Semantic Collisions").Sprint("No search events captured yet. Collisions appear after align_tools queries.")
	}

	// ── Bidding Table ──────────────────────────────────────────────────────
	eventsRaw, _ := raw["events"].([]any)
	rows := [][]string{
		{"Query", "S1", "S2", "Gap", "Status"},
	}
	for _, e := range eventsRaw {
		evt, ok := e.(map[string]any)
		if !ok {
			continue
		}
		query := str(evt, "query")
		if len(query) > 30 {
			query = query[:30] + "..."
		}
		s1 := str(evt, "s1_urn")
		s2 := str(evt, "s2_urn")
		gap := numF64(evt, "gap")
		gapStr := fmt.Sprintf("%.4f", gap)
		status := pterm.Green("● Healthy")
		if gap < 0.05 {
			status = pterm.Red("● Collision")
			gapStr = pterm.Red(gapStr)
		} else if gap < 0.10 {
			status = pterm.Yellow("● Narrow")
			gapStr = pterm.Yellow(gapStr)
		} else {
			gapStr = pterm.Green(gapStr)
		}
		rows = append(rows, []string{query, s1, s2, gapStr, status})
	}
	biddingContent := "No events."
	if len(rows) > 1 {
		table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
		biddingContent = table
	}
	biddingBox := pterm.DefaultBox.WithTitle("Bidding Table (Recent Searches)").Sprint(biddingContent)

	// ── Collision Pairs ───────────────────────────────────────────────────
	pairsRaw, _ := raw["top_pairs"].([]any)
	pairsContent := "No collision pairs detected."
	if len(pairsRaw) > 0 {
		pairRows := [][]string{{"Tool A", "Tool B", "Count"}}
		for _, p := range pairsRaw {
			pair, ok := p.(map[string]any)
			if !ok {
				continue
			}
			pairRows = append(pairRows, []string{
				str(pair, "urn_a"),
				str(pair, "urn_b"),
				pterm.Red(str(pair, "count")),
			})
		}
		if len(pairRows) > 1 {
			pairsTable, _ := pterm.DefaultTable.WithHasHeader().WithData(pairRows).Srender()
			pairsContent = pairsTable
		}
	}
	pairsBox := pterm.DefaultBox.WithTitle("Top Collision Pairs").Sprint(pairsContent)

	// ── Trend Indicator ────────────────────────────────────────────────────
	totalEvents := str(raw, "total_events")
	totalCollisions := str(raw, "total_collisions")
	avgGap := numF64(raw, "avg_gap")
	trend := str(raw, "trend")

	trendIcon := "➡️"
	switch trend {
	case "improving":
		trendIcon = pterm.Green("▲ Improving")
	case "degrading":
		trendIcon = pterm.Red("▼ Degrading")
	case "stable":
		trendIcon = pterm.Cyan("● Stable")
	default:
		trendIcon = pterm.Gray("◌ " + trend)
	}

	avgGapStr := fmt.Sprintf("%.4f", avgGap)
	if avgGap < 0.05 {
		avgGapStr = pterm.Red(avgGapStr)
	} else if avgGap < 0.10 {
		avgGapStr = pterm.Yellow(avgGapStr)
	} else {
		avgGapStr = pterm.Green(avgGapStr)
	}

	trendRows := [][]string{
		{"Metric", "Value"},
		{"Total Search Events", totalEvents},
		{"Total Collisions", totalCollisions},
		{"Avg Confidence Gap", avgGapStr},
		{"Trend Direction", trendIcon},
	}
	trendTable, _ := pterm.DefaultTable.WithHasHeader().WithData(trendRows).Srender()
	trendBox := pterm.DefaultBox.WithTitle("Trend Indicator").Sprint(trendTable)

	panels, _ := pterm.DefaultPanel.WithPanels(pterm.Panels{
		{{Data: biddingBox}},
		{{Data: pairsBox}, {Data: trendBox}},
	}).Srender()

	return panels
}

// ── Tab 8: Spans (Waterfall) ────────────────────────────────────────────────

func renderSpans(logs []string, uiState *InternalUIState) string {
	if len(logs) == 0 {
		return pterm.DefaultBox.WithTitle("Distributed Spans").Sprint("No backplane events captured.")
	}

	type SpanNode struct {
		TraceID   string
		ParentID  string
		Server    string
		Tool      string
		StartTime int64
		LatencyMs int64
		Children  []*SpanNode
	}

	nodes := make(map[string]*SpanNode)
	var rootNodes []*SpanNode

	for _, l := range logs {
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err != nil {
			continue
		}
		typ := str(evt, "type")
		if typ != "SPAN_START" && typ != "SPAN_END" {
			continue
		}

		tid := str(evt, "trace_id")
		node, exists := nodes[tid]
		if !exists {
			node = &SpanNode{TraceID: tid}
			nodes[tid] = node
		}

		switch typ {
		case "SPAN_START":
			node.Server = str(evt, "server")
			node.Tool = str(evt, "tool")
			node.ParentID = str(evt, "parent_id")
			node.StartTime = numI64(evt, "start_time")
		case "SPAN_END":
			node.LatencyMs = numI64(evt, "latency_ms")
		}
	}

	for _, node := range nodes {
		if node.ParentID != "" && node.ParentID != "-" {
			if parent, hasParent := nodes[node.ParentID]; hasParent {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		rootNodes = append(rootNodes, node)
	}

	sort.Slice(rootNodes, func(i, j int) bool {
		return rootNodes[i].StartTime < rootNodes[j].StartTime
	})

	var visibleNodes []string
	selTraceID, spansExpand, spansFocus := uiState.GetSnapshot()

	var printNode func(node *SpanNode, indent string, isLast bool) string
	printNode = func(node *SpanNode, indent string, isLast bool) string {
		visibleNodes = append(visibleNodes, node.TraceID)

		marker := "├── "
		if isLast {
			marker = "└── "
		}
		if indent == "" {
			marker = "▶ "
		}

		expandMarker := ""
		if len(node.Children) > 0 {
			if spansExpand[node.TraceID] {
				expandMarker = "[-] "
			} else {
				expandMarker = "[+] "
			}
		}

		latencyStr := pterm.Cyan("...executing")
		if node.LatencyMs > 0 {
			latencyStr = pterm.Green(fmt.Sprintf("%dms", node.LatencyMs))
		}

		linePrefix := indent + marker + expandMarker
		nodeText := fmt.Sprintf("[%s] %s %v\n", pterm.LightMagenta(node.Server), pterm.Yellow(node.Tool), latencyStr)

		if spansFocus && node.TraceID == selTraceID {
			linePrefix = pterm.BgWhite.Sprint(pterm.Black(" >> ")) + linePrefix
			nodeText = pterm.BgDarkGray.Sprint(fmt.Sprintf("[%s] %s %v", node.Server, node.Tool, latencyStr)) + "\n"
		} else {
			linePrefix = "    " + linePrefix
		}

		line := linePrefix + nodeText

		if len(node.Children) > 0 && !spansExpand[node.TraceID] {
			return line
		}

		sort.Slice(node.Children, func(i, j int) bool {
			return node.Children[i].StartTime < node.Children[j].StartTime
		})

		childIndent := indent
		if indent == "" {
			childIndent = "  "
		} else {
			if isLast {
				childIndent += "    "
			} else {
				childIndent += "│   "
			}
		}

		for i, child := range node.Children {
			line += printNode(child, childIndent, i == len(node.Children)-1)
		}
		return line
	}

	var sb strings.Builder
	for _, root := range rootNodes {
		val := printNode(root, "", true)
		if len(val) > 0 {
			sb.WriteString(val + "\n")
		}
	}

	uiState.UpdateNodes(visibleNodes)

	content := sb.String()
	if strings.TrimSpace(content) == "" {
		content = "No structured tracing spans are currently executing or resolved within the ring buffer timeout natively."
	}

	return pterm.DefaultBox.WithTitle("Spans (Waterfall)").Sprint(content)
}

// ── Tab 16: Token-Value ─────────────────────────────────────────────────────

func renderTokenValue(snapshot map[string]any) string {
	optRaw, ok := snapshot["opt_metrics"].(map[string]any)
	if !ok || len(optRaw) == 0 {
		return pterm.DefaultBox.WithTitle("Token-Value").Sprint("Waiting for proxy token data...")
	}

	rawBytes := numI64(optRaw, "total_raw_bytes")
	squeezedBytes := numI64(optRaw, "total_squeezed_bytes")

	var efficiency float64
	efficiencyStr := "0.0%"
	savedBytes := int64(0)

	if rawBytes > 0 {
		efficiency = 1.0 - (float64(squeezedBytes) / float64(rawBytes))
		efficiencyStr = fmt.Sprintf("%.2f%%", efficiency*100)
		savedBytes = rawBytes - squeezedBytes
		if efficiency > 0.5 {
			efficiencyStr = pterm.Green(efficiencyStr)
		} else if efficiency > 0.1 {
			efficiencyStr = pterm.Yellow(efficiencyStr)
		} else {
			efficiencyStr = pterm.Red(efficiencyStr)
		}
	}

	rows := [][]string{
		{"Metric", "Value"},
		{"Total Raw Bytes", fmt.Sprintf("%d B", rawBytes)},
		{"Total Minified Bytes", fmt.Sprintf("%d B", squeezedBytes)},
		{"Bytes Saved", fmt.Sprintf("%d B", savedBytes)},
		{"Squeeze Efficiency Ratio", efficiencyStr},
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	return pterm.DefaultBox.WithTitle("Proxy Compression Efficiency (Token-Value)").Sprint(table)
}

// ── Tab 17: RAG ─────────────────────────────────────────────────────────────

func renderRAG(snapshot map[string]any) string {
	searchRaw, ok := snapshot["search"].(map[string]any)
	if !ok || len(searchRaw) == 0 {
		return pterm.DefaultBox.WithTitle("RAG Confidence Map").Sprint("Waiting for RAG vector telemetry...")
	}

	totalSearches := numI64(searchRaw, "vector_searches")
	totalConfidenceTokens := numF64(searchRaw, "total_confidence_score")

	averageConfidence := 0.0
	avgConfStr := "N/A"
	alertStr := pterm.Green("● Healthy")

	if totalSearches > 0 {
		averageConfidence = totalConfidenceTokens / float64(totalSearches)
		avgConfStr = fmt.Sprintf("%.4f", averageConfidence)
		if averageConfidence < 0.60 {
			alertStr = pterm.Red("● DANGER (Hallucination Risk)")
			avgConfStr = pterm.Red(avgConfStr)
		} else if averageConfidence < 0.80 {
			alertStr = pterm.Yellow("● CAUTION (Sub-optimal Alignment)")
			avgConfStr = pterm.Yellow(avgConfStr)
		} else {
			avgConfStr = pterm.Green(avgConfStr)
		}
	}

	rows := [][]string{
		{"Metric", "Value"},
		{"Vector Operations", fmt.Sprintf("%d", totalSearches)},
		{"Cumulative Target Distance", fmt.Sprintf("%.4f", totalConfidenceTokens)},
		{"Average Composition Confidence", avgConfStr},
		{"RAG Structural Safety Status", alertStr},
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(rows).Srender()
	return pterm.DefaultBox.WithTitle("Vector Search Confidence Map (RAG)").Sprint(table)
}
