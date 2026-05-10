package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/ui"
)

var dashboardCmd = &cobra.Command{
	Use:   "dash",
	Short: "Launch the observability dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		ui.EnableVirtualTerminalProcessing()
		runInteractiveDashboard()
	},
}

type metricsMsg struct {
	Keys       int
	DBSize     int64
	Sessions   []db.SessionState
	Latencies  []string
	Hashes     []string
	Hydrations []string
}

type model struct {
	activeTab int
	keys      int
	dbSize    int64
	sessions  []db.SessionState
	latencies []string
	hashes    []string
	hydrations []string
}

var (
	tabStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 2)

	activeTabStyle = tabStyle.
			Border(lipgloss.NormalBorder(), true, true, false, true).
			BorderForeground(lipgloss.Color("62")).
			Foreground(lipgloss.Color("62"))

	windowStyle = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Border(lipgloss.NormalBorder())
)

func runInteractiveDashboard() {
	_ = config.LoadConfig()
	dbPath := viper.GetString("server.db_path")
	if dbPath == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		dbPath = filepath.Join(cacheDir, "mcp-server-magicdev", "session.db")
	} else if dbPath != ":memory:" {
		dbPath = filepath.Clean(filepath.FromSlash(dbPath))
	}
	cacheDir, _ := os.UserCacheDir()
	logPath := filepath.Join(cacheDir, "mcp-server-magicdev", "magicdev-output.log")

	m := model{}
	p := tea.NewProgram(m, tea.WithAltScreen())
	
	// Start polling
	go func() {
		for {
			metrics := fetchMetrics(dbPath, logPath)
			p.Send(metrics)
			time.Sleep(5 * time.Second)
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running dashboard: %v\n", err)
		os.Exit(1)
	}
}

func fetchMetrics(dbPath, logPath string) metricsMsg {
	var msg metricsMsg
	
	// Safely copy DB to temp file to bypass locks
	tempPath := filepath.Join(os.TempDir(), "magicdev-dash-snapshot.db")
	in, err := os.Open(dbPath)
	if err == nil {
		out, err := os.Create(tempPath)
		if err == nil {
			io.Copy(out, in)
			out.Close()
			in.Close()

			if store, err := db.InitStoreWithPath(tempPath); err == nil {
				msg.Keys = store.DBEntries()
				msg.DBSize = store.DBSize()
				msg.Sessions, _ = store.ListSessions()
				store.Close()
			}
			os.Remove(tempPath)
		} else {
			in.Close()
		}
	}

	// Parse logs for telemetry
	if file, err := os.Open(logPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		// keep last 50 lines
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if len(lines) > 500 {
				lines = lines[100:]
			}
		}
		
		for _, line := range lines {
			if strings.Contains(line, "bunt_latency_us") {
				msg.Latencies = append(msg.Latencies, extractJSONValue(line, "operation")+" "+extractJSONValue(line, "bunt_latency_us")+"µs")
			}
			if strings.Contains(line, "Session state integrity hash") {
				msg.Hashes = append(msg.Hashes, extractJSONValue(line, "step")+" "+extractJSONValue(line, "sha256")[:16]+"...")
			}
			if strings.Contains(line, "Payload completeness evaluated") || strings.Contains(line, "TELEMETRY WARNING") {
				msg.Hydrations = append(msg.Hydrations, extractJSONValue(line, "step")+" "+extractJSONValue(line, "ratio"))
			}
		}
		if len(msg.Latencies) > 10 { msg.Latencies = msg.Latencies[len(msg.Latencies)-10:] }
		if len(msg.Hashes) > 10 { msg.Hashes = msg.Hashes[len(msg.Hashes)-10:] }
		if len(msg.Hydrations) > 10 { msg.Hydrations = msg.Hydrations[len(msg.Hydrations)-10:] }
	}

	return msg
}

func extractJSONValue(line, key string) string {
	parts := strings.Split(line, "\""+key+"\":")
	if len(parts) > 1 {
		val := parts[1]
		if strings.HasPrefix(val, "\"") {
			return strings.Split(val[1:], "\"")[0]
		}
		return strings.Split(strings.Split(val, ",")[0], "}")[0]
	}
	return ""
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % 2
		case "shift+tab", "left":
			m.activeTab = (m.activeTab - 1) % 2
			if m.activeTab < 0 {
				m.activeTab = 1
			}
		}
	case metricsMsg:
		m.keys = msg.Keys
		m.dbSize = msg.DBSize
		m.sessions = msg.Sessions
		m.latencies = msg.Latencies
		m.hashes = msg.Hashes
		m.hydrations = msg.Hydrations
	}
	return m, nil
}

func (m model) View() string {
	tabs := []string{"Database Health", "Pipeline Flow"}
	var renderedTabs []string

	for i, t := range tabs {
		if i == m.activeTab {
			renderedTabs = append(renderedTabs, activeTabStyle.Render(t))
		} else {
			renderedTabs = append(renderedTabs, tabStyle.Render(t))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	var content string
	if m.activeTab == 0 {
		content = fmt.Sprintf("BuntDB Storage Size: %d bytes\nTotal Keys: %d\n\nRecent Transaction Latencies:\n", m.dbSize, m.keys)
		for _, l := range m.latencies {
			content += " - " + l + "\n"
		}
	} else {
		content = "Pipeline Flow Telemetry:\n\n"
		content += "Active Session Phase Dwell Times:\n"
		for _, s := range m.sessions {
			if s.CurrentStep != "" {
				if t, ok := s.StepTimings[s.CurrentStep]; ok {
					if startedAt, err := time.Parse(time.RFC3339, t.StartedAt); err == nil {
						content += fmt.Sprintf(" - [%s] %s: %s\n", s.SessionID[:8], s.CurrentStep, time.Since(startedAt).Round(time.Second))
					}
				}
			}
		}
		content += "\nInter-Tool Integrity Hashes:\n"
		for _, h := range m.hashes {
			content += " - " + h + "\n"
		}
		content += "\nPayload Completeness Ratios:\n"
		for _, h := range m.hydrations {
			content += " - " + h + "\n"
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, row, windowStyle.Render(content))
}
