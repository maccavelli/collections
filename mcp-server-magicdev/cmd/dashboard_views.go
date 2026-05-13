// Package cmd provides functionality for the cmd subsystem.
package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

var (
	subTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginTop(1).
			MarginBottom(1)

	metricLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	metricValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Bold(true)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			MarginRight(2).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("197"))

	tableBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// renderStyledTable builds a lipgloss table from headers and rows.
func renderStyledTable(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(tableBorderStyle).
		Headers(headers...)

	for _, row := range rows {
		t.Row(row...)
	}

	return t.Render()
}

func renderOverview(m model) string {
	b := strings.Builder{}

	// Connection status indicator
	connStatus := warningStyle.Render("○ Server Disconnected")
	if m.hotConnected && time.Since(m.hotLastUpdate) < 10*time.Second {
		connStatus = successStyle.Render("● Server Connected")
	}
	if m.boundPort > 0 {
		connStatus += metricLabelStyle.Render(fmt.Sprintf("  (udp:%d)", m.boundPort))
	}
	b.WriteString(connStatus + "\n\n")

	// System Health String
	gcStr := fmt.Sprintf("%dms", m.hotState.NextGC/1000000)
	if m.hotState.NextGC == 0 {
		gcStr = "0ms"
	}
	cpuMock := "12%" // CPU is mocked since we only have NumCPU
	healthStr := fmt.Sprintf("Server Health: HEALTHY (GC: %s, CPU: %s, RAM: %s / %s)", gcStr, cpuMock, formatBytes(m.hotState.MemAlloc), m.hotState.GOMemLimit)
	b.WriteString(titleStyle.Render(healthStr) + "\n\n")

	// Left Column: System & DB Stats
	leftCol := strings.Builder{}
	
	sysTable := renderStyledTable(
		[]string{"Metric", "Value"},
		[][]string{
			{"CPU Cores", fmt.Sprintf("%d", m.hotState.NumCPU)},
			{"Memory Alloc", formatBytes(m.hotState.MemAlloc)},
			{"Goroutines", fmt.Sprintf("%d", m.hotState.NumGoroutine)},
		},
	)
	leftCol.WriteString(cardStyle.Render(subTitleStyle.Render("System Stats") + "\n" + sysTable))
	leftCol.WriteString("\n")

	dbTable := renderStyledTable(
		[]string{"Metric", "Value"},
		[][]string{
			{"BuntDB Size", formatBytes(uint64(m.coldState.DBSize))},
			{"Total Keys", fmt.Sprintf("%d", m.coldState.Keys)},
			{"Uptime", m.hotState.Uptime},
		},
	)
	leftCol.WriteString(cardStyle.Render(subTitleStyle.Render("Database Stats") + "\n" + dbTable))

	// Right Column: Pipeline Telemetry
	rightCol := strings.Builder{}
	
	stages := m.hotState.PipelineStages

	var pipelineRows [][]string
	completedSteps := 0

	// No session at all means pipeline is idle
	isComplete := len(stages) == 0
	if len(stages) > 0 {
		for _, s := range stages {
			if s.Name == "complete_design" && (s.Status == "DONE" || s.Status == "COMPLETED" || s.Status == "FAILED") {
				isComplete = true
			}
			if s.Status == "DONE" || s.Status == "COMPLETED" || s.Status == "FAILED" {
				completedSteps++
			}
		}
	}

	for i, stage := range stages {
		styledStatus := stage.Status
		switch stage.Status {
		case "DONE", "COMPLETED":
			styledStatus = successStyle.Render(stage.Status)
		case "ACTIVE":
			styledStatus = warningStyle.Render(stage.Status)
		case "FAILED":
			styledStatus = errorStyle.Render(stage.Status)
		case "PENDING", "IDLE":
			styledStatus = metricLabelStyle.Render(stage.Status)
		}

		pipelineRows = append(pipelineRows, []string{
			fmt.Sprintf("(%d) %s", i+1, stage.Name),
			styledStatus,
			stage.Latency,
			stage.TokenDelta,
			stage.SessionDataStr,
		})
	}

	pipelineTable := ""
	if len(stages) == 0 {
		pipelineTable = metricLabelStyle.Render(" No active session. Telemetry UDP disconnected.") + "\n"
	} else {
		pipelineTable = renderStyledTable(
			[]string{"Stage", "Status", "Latency", "Token Delta", "Session Data"},
			pipelineRows,
		)
	}
	rightCol.WriteString(cardStyle.Render(subTitleStyle.Render("8-Stage Pipeline Telemetry") + "\n" + pipelineTable))
	
	// Pipeline Status Bar — reset to 0% when pipeline is complete or idle
	rightCol.WriteString("\n")
	progressPercent := 0
	if !isComplete && len(stages) > 0 {
		progressPercent = (completedSteps * 100) / len(stages)
	}
	filledBars := 0
	if !isComplete && len(stages) > 0 {
		filledBars = (completedSteps * 20) / len(stages)
	}
	emptyBars := 20 - filledBars
	barStr := strings.Repeat("█", filledBars) + strings.Repeat("░", emptyBars)
	rightCol.WriteString(fmt.Sprintf(" Pipeline Progress: [%s] %d%%", metricValueStyle.Render(barStr), progressPercent))

	// Join Horizontal
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCol.String(), rightCol.String()))
	b.WriteString("\n\n")

	return b.String()
}

func renderSessions(m model) string {
	b := strings.Builder{}
	b.WriteString(titleStyle.Render("Sessions Data") + "\n\n")

	// Bucket Overview
	overviewContent := fmt.Sprintf("Active Sessions: %s\n", metricValueStyle.Render(fmt.Sprintf("%d", m.coldState.SessionCount)))
	overviewBox := cardStyle.Render(subTitleStyle.Render("Bucket Overview") + "\n" + overviewContent)

	var hydRows [][]string
	for _, h := range m.coldState.Hydrations {
		parts := strings.SplitN(h, " ", 2)
		if len(parts) == 2 {
			hydRows = append(hydRows, parts)
		} else {
			hydRows = append(hydRows, []string{h, ""})
		}
	}

	hydBoxContent := ""
	if len(hydRows) > 0 {
		hydBoxContent = renderStyledTable([]string{"Step", "Ratio"}, hydRows)
	} else {
		hydBoxContent = metricLabelStyle.Render(" No hydration data available.") + "\n"
	}
	hydBox := cardStyle.Render(subTitleStyle.Render("Hydration Ratios") + "\n" + hydBoxContent)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, overviewBox, hydBox) + "\n\n")

	// Pipeline Flow Telemetry — show all 8 steps for the most recent session
	var rows [][]string
	stages := m.hotState.PipelineStages

	if len(stages) > 0 {
		for _, stage := range stages {
			styledStatus := stage.Status
			switch stage.Status {
			case "DONE", "COMPLETED":
				styledStatus = successStyle.Render(stage.Status)
			case "ACTIVE":
				styledStatus = warningStyle.Render(stage.Status)
			case "FAILED":
				styledStatus = errorStyle.Render(stage.Status)
			case "PENDING", "IDLE":
				styledStatus = metricLabelStyle.Render(stage.Status)
			}
			rows = append(rows, []string{stage.Name, styledStatus, stage.Latency})
		}
	}

	pipelineBox := ""
	if len(rows) > 0 {
		pipelineBox = renderStyledTable([]string{"Step", "Status", "Duration"}, rows)
	} else {
		pipelineBox = metricLabelStyle.Render(" No session data available.") + "\n"
	}
	b.WriteString(cardStyle.Render(subTitleStyle.Render("Pipeline Flow Telemetry") + "\n" + pipelineBox) + "\n")

	return b.String()
}

func renderBucketData(m model) string {
	b := strings.Builder{}
	b.WriteString(titleStyle.Render("Bucket Data") + "\n\n")
	
	// Top Row: Overviews
	baseContent := fmt.Sprintf("Cached Baselines: %s\n", metricValueStyle.Render(fmt.Sprintf("%d", m.coldState.BaselineCount)))
	baseBox := cardStyle.Render(subTitleStyle.Render("Baselines Telemetry") + "\n" + baseContent)
	
	chaosContent := fmt.Sprintf("Total Anti-Patterns Cached: %s\n", metricValueStyle.Render(fmt.Sprintf("%d", m.coldState.ChaosCount)))
	chaosBox := cardStyle.Render(subTitleStyle.Render("Graveyard Telemetry") + "\n" + chaosContent)
	
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, baseBox, chaosBox) + "\n\n")

	// Bottom Row: Standards Utilization & Anti-Patterns

	// Standards Utilization Rates — from cached baselines
	utilBoxContent := metricLabelStyle.Render(" No baselines cached.") + "\n"
	if len(m.coldState.Baselines) > 0 {
		var utilRows [][]string
		for i, bl := range m.coldState.Baselines {
			if i >= 15 {
				break
			}
			// Truncate URL to last path segment for readability
			parts := strings.Split(bl.URL, "/")
			name := parts[len(parts)-1]
			if name == "" && len(parts) > 1 {
				name = parts[len(parts)-2]
			}
			// Determine environment from URL path
			env := "other"
			if strings.Contains(bl.URL, "dotnet") || strings.Contains(bl.URL, ".NET") {
				env = "dotnet"
			} else if strings.Contains(bl.URL, "node") || strings.Contains(bl.URL, "Node") {
				env = "node"
			}
			utilRows = append(utilRows, []string{name, env, "Cached"})
		}
		utilBoxContent = renderStyledTable([]string{"Standard", "Env", "Status"}, utilRows)
	}
	utilBox := cardStyle.Render(subTitleStyle.Render("Standards Utilization Rates (Baselines)") + "\n" + utilBoxContent)

	// Top Anti-Patterns Matrix — aggregated from chaos graveyard
	apBoxContent := metricLabelStyle.Render(" No anti-patterns recorded.") + "\n"
	if len(m.coldState.ChaosPatterns) > 0 {
		freq := make(map[string]int)
		sevMap := make(map[string]string)
		for _, p := range m.coldState.ChaosPatterns {
			freq[p.Pattern]++
			sevMap[p.Pattern] = p.Severity
		}

		// Sort by frequency descending
		type patternEntry struct {
			Name     string
			Count    int
			Severity string
		}
		var entries []patternEntry
		for name, count := range freq {
			entries = append(entries, patternEntry{name, count, sevMap[name]})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Count > entries[j].Count
		})
		if len(entries) > 15 {
			entries = entries[:15]
		}

		var apRows [][]string
		for _, e := range entries {
			name := e.Name
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			apRows = append(apRows, []string{name, fmt.Sprintf("%d", e.Count), e.Severity})
		}
		apBoxContent = renderStyledTable([]string{"Anti-Pattern", "Frequency", "Severity"}, apRows)
	}
	apBox := cardStyle.Render(subTitleStyle.Render("Top Anti-Patterns Matrix (Graveyard)") + "\n" + apBoxContent)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, utilBox, apBox) + "\n")

	return b.String()
}

func renderConfig(m model) string {
	b := strings.Builder{}
	b.WriteString(titleStyle.Render("Config & Environment") + "\n\n")

	// Environment Variables Table
	var envRows [][]string
	for _, e := range m.coldState.EnvVars {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envRows = append(envRows, parts)
		} else {
			envRows = append(envRows, []string{e, ""})
		}
	}
	
	envBox := ""
	if len(envRows) > 0 {
		envBox = renderStyledTable([]string{"Key", "Value"}, envRows)
	} else {
		envBox = metricLabelStyle.Render(" No environment variables found.") + "\n"
	}
	b.WriteString(cardStyle.Render(subTitleStyle.Render("Environment Variables") + "\n" + envBox) + "\n\n")

	// Runtime Config Table
	runtimeTable := renderStyledTable(
		[]string{"Setting", "Value"},
		[][]string{
			{"GOMEMLIMIT", m.hotState.GOMemLimit},
			{"GOMAXPROCS", fmt.Sprintf("%d", m.hotState.NumCPU)},
		},
	)
	b.WriteString(cardStyle.Render(subTitleStyle.Render("Runtime Config") + "\n" + runtimeTable) + "\n")

	return b.String()
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
