// Package cmd provides functionality for the cmd subsystem.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/telemetry"
	"mcp-server-magicdev/internal/ui"
)

var startTime = time.Now()

var dashboardCmd = &cobra.Command{
	Use:   "dash",
	Short: "Launch the observability dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		ui.EnableVirtualTerminalProcessing()
		runInteractiveDashboard()
	},
}

type metricsMsg struct {
	// DB Metrics
	Keys          int
	DBSize        int64
	SessionCount  int
	BaselineCount int
	ChaosCount    int
	Sessions      []db.SessionState

	// Telemetry Logs
	Hydrations []string

	// Bucket Data
	Baselines     []db.BaselineMeta
	ChaosPatterns []db.ChaosRejection

	// Env
	EnvVars []string
}

type udpMetricsMsg telemetry.MetricPayload

type model struct {
	activeTab     int
	coldState     metricsMsg
	hotState      udpMetricsMsg
	boundPort     int
	hotConnected  bool
	hotLastUpdate time.Time
}

const (
	tabOverview = iota
	tabSessions
	tabBucketData
	tabConfig
	tabQuit
)

var navItems = []string{
	"Overview",
	"Sessions Data",
	"Bucket Data",
	"Config & Environment",
	"Quit",
}

var (
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 2).
			Width(30)

	navItemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	activeNavItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Bold(true)

	windowStyle = lipgloss.NewStyle().
			Padding(1, 4)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginBottom(1)
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
	m := model{}

	p := tea.NewProgram(m, tea.WithAltScreen())

	// Start persistent UDP client goroutine with auto-reconnect (HOT STATE)
	go func() {
		conn, boundPort := udpSweepPorts()
		if conn == nil {
			slog.Warn("could not connect to any telemetry port; will retry")
		} else {
			p.Send(reconnectMsg{port: boundPort})
		}

		buf := make([]byte, 4096)
		pingTicker := time.NewTicker(telemetry.EmissionInterval)
		defer pingTicker.Stop()

		const maxConsecutiveFailures = 6
		consecutiveFailures := 0
		backoff := 2 * time.Second
		const maxBackoff = 10 * time.Second

		for range pingTicker.C {
			if conn == nil {
				time.Sleep(backoff)
				conn, boundPort = udpSweepPorts()
				if conn != nil {
					consecutiveFailures = 0
					backoff = 2 * time.Second
					p.Send(reconnectMsg{port: boundPort})
					slog.Info("telemetry reconnected", "port", boundPort)
				} else {
					backoff = min(backoff*2, maxBackoff)
				}
				continue
			}

			_, err := conn.Write([]byte{0x01})
			if err != nil {
				if isClosedErr(err) {
					return
				}
				consecutiveFailures++
				if consecutiveFailures >= maxConsecutiveFailures {
					slog.Warn("telemetry connection lost, initiating re-sweep",
						"failures", consecutiveFailures)
					conn.Close()
					conn = nil
				}
				continue
			}

			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				if isClosedErr(err) {
					return
				}
				consecutiveFailures++
				if consecutiveFailures >= maxConsecutiveFailures {
					slog.Warn("telemetry connection lost, initiating re-sweep",
						"failures", consecutiveFailures)
					conn.Close()
					conn = nil
				}
				continue
			}

			consecutiveFailures = 0
			var payload telemetry.MetricPayload
			if json.Unmarshal(buf[:n], &payload) == nil {
				p.Send(udpMetricsMsg(payload))
			}
		}
	}()

	// 3. Start polling DB (COLD STATE)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		var dbMu sync.Mutex

		// Initial load
		metrics := ReadDashboardSnapshot(dbPath)
		p.Send(metrics)

		for range ticker.C {
			if dbMu.TryLock() {
				// We don't block if a file-copy is still running; we just skip the tick.
				go func() {
					defer dbMu.Unlock()
					m := ReadDashboardSnapshot(dbPath)
					p.Send(m)
				}()
			}
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running dashboard: %v\n", err)
		os.Exit(1)
	}
}

// ReadDashboardSnapshot performs the ReadDashboardSnapshot operation.
func ReadDashboardSnapshot(dbPath string) metricsMsg {
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
				msg.SessionCount = store.SessionCount()
				msg.BaselineCount = store.BaselineCount()
				msg.ChaosCount = store.ChaosGraveyardCount()
				msg.Sessions, _ = store.ListSessions()
				msg.Baselines, _ = store.ListBaselines()
				msg.ChaosPatterns = store.ListAllChaosGraveyards()
				store.Close()
			}
			os.Remove(tempPath)
		} else {
			in.Close()
		}
	} else if os.IsNotExist(err) {
		// Graceful degradation: db doesn't exist yet
	}

	envMappings := []struct {
		envKey   string
		viperKey string
	}{
		{"JIRA_URL", "jira.url"},
		{"CONFLUENCE_URL", "confluence.url"},
		{"GITLAB_URL", "git.server_url"},
		{"MAGICDEV_DB_PATH", "server.db_path"},
	}

	for _, mapping := range envMappings {
		if v := viper.GetString(mapping.viperKey); v != "" {
			msg.EnvVars = append(msg.EnvVars, fmt.Sprintf("%s=%s", mapping.envKey, v))
		} else {
			msg.EnvVars = append(msg.EnvVars, fmt.Sprintf("%s=(unset)", mapping.envKey))
		}
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

// isClosedErr checks if the error indicates a closed socket.
func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed")
}

// reconnectMsg notifies the BubbleTea program of a port change.
type reconnectMsg struct {
	port int
}

// udpDialAndValidate connects to a port and verifies the server responds.
func udpDialAndValidate(port int) *net.UDPConn {
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil
	}
	_, _ = c.Write([]byte{0x01})
	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 4096)
	_, err = c.Read(buf)
	if err != nil {
		c.Close()
		return nil
	}
	return c
}

// udpSweepPorts attempts to connect to the first responding telemetry port.
func udpSweepPorts() (*net.UDPConn, int) {
	for _, port := range telemetry.TelemetryPorts {
		if c := udpDialAndValidate(port); c != nil {
			return c, port
		}
	}
	return nil, 0
}

// Init performs the Init operation.
func (m model) Init() tea.Cmd {
	return nil
}

// Update performs the Update operation.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = len(navItems) - 1
			}
		case "down", "j":
			m.activeTab++
			if m.activeTab >= len(navItems) {
				m.activeTab = 0
			}
		case "enter":
			if m.activeTab == tabQuit {
				return m, tea.Quit
			}
		}
	case metricsMsg:
		m.coldState = msg
	case udpMetricsMsg:
		m.hotState = msg
		m.hotConnected = true
		m.hotLastUpdate = time.Now()
	case reconnectMsg:
		m.boundPort = msg.port
	}
	return m, nil
}

// View performs the View operation.
func (m model) View() string {
	var navLines []string
	navLines = append(navLines, titleStyle.Render("MagicDev Dash"))
	navLines = append(navLines, "") // separator

	for i, item := range navItems {
		if i == m.activeTab {
			navLines = append(navLines, activeNavItemStyle.Render("> "+item))
		} else {
			navLines = append(navLines, navItemStyle.Render("  "+item))
		}
	}

	sidebar := sidebarStyle.Render(strings.Join(navLines, "\n"))

	var content string
	switch m.activeTab {
	case tabOverview:
		content = renderOverview(m)
	case tabSessions:
		content = renderSessions(m)
	case tabBucketData:
		content = renderBucketData(m)
	case tabConfig:
		content = renderConfig(m)
	case tabQuit:
		content = titleStyle.Render("Quit") + "\n\nPress Enter to exit the dashboard."
	}

	mainView := windowStyle.Render(content)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainView)
}
