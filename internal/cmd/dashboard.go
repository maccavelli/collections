package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"mcp-server-socratic-thinker/internal/telemetry"
)

var dashboardCmd = &cobra.Command{
	Use:     "dashboard",
	Aliases: []string{"dash"},
	Short:   "View the telemetry dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Connect to serve process's UDP telemetry listener
		var conn *net.UDPConn
		var boundPort int
		for _, port := range telemetry.TelemetryPorts {
			addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
			c, err := net.DialUDP("udp", nil, addr)
			if err == nil {
				conn = c
				boundPort = port
				break
			}
		}

		m := initialModel()
		if conn != nil {
			m.boundPort = boundPort
		} else {
			slog.Warn("could not connect to any telemetry port; session data will be unavailable")
		}

		// 2. Create program
		p := tea.NewProgram(m, tea.WithAltScreen())

		// 3. Start persistent UDP client goroutine (ping + receive)
		if conn != nil {
			go func() {
				defer conn.Close()
				buf := make([]byte, 4096)
				pingTicker := time.NewTicker(telemetry.EmissionInterval)
				defer pingTicker.Stop()

				for range pingTicker.C {
					// Send a 1-byte ping to register our address with the serve process
					_, err := conn.Write([]byte{0x01})
					if err != nil {
						if isClosedErr(err) {
							return
						}
						continue
					}

					// Read the response (MetricPayload JSON)
					conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					n, err := conn.Read(buf)
					if err != nil {
						if isClosedErr(err) {
							return
						}
						continue
					}
					var payload telemetry.MetricPayload
					if json.Unmarshal(buf[:n], &payload) == nil {
						p.Send(sessionMsg(payload))
					}
				}
			}()
		}

		// 4. Start self-polling goroutine for system metrics
		go func() {
			startTime := time.Now()
			for {
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				p.Send(systemMsg{
					UptimeSeconds:    int64(time.Since(startTime).Seconds()),
					MemoryAllocBytes: memStats.Alloc,
					ActiveGoroutines: runtime.NumGoroutine(),
					GCPauseNs:        memStats.PauseTotalNs,
				})
				time.Sleep(1 * time.Second)
			}
		}()

		// 5. Run blocks until user quits
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running dashboard: %v\n", err)
			os.Exit(1)
		}
	},
}

// isClosedErr checks if the error indicates a closed socket.
func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed")
}

// Styling Variables matching MagicDev
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

	subTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginTop(1).
			MarginBottom(1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			MarginRight(2).
			MarginBottom(1)

	metricLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	metricValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Bold(true)

	successStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warningStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	tableBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// renderStyledTable builds a lipgloss table from headers and rows.
func renderStyledTable(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(tableBorderStyle).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 0 {
				return lipgloss.NewStyle().Width(20)
			}
			if col == 1 {
				return lipgloss.NewStyle().Width(14)
			}
			return lipgloss.NewStyle()
		})

	for _, row := range rows {
		t.Row(row...)
	}

	return t.Render()
}

const (
	tabSummary = iota
	tabQuit
)

var navItems = []string{
	"Summary",
	"Quit",
}

// systemMsg carries self-polled system metrics from the dashboard process.
type systemMsg struct {
	UptimeSeconds    int64
	MemoryAllocBytes uint64
	ActiveGoroutines int
	GCPauseNs        uint64
}

// sessionMsg carries UDP-received session metrics from the serve process.
type sessionMsg telemetry.MetricPayload

type model struct {
	activeTab int
	width     int
	height    int

	// System metrics (self-polled, always live)
	sysUptime     int64
	sysMemAlloc   uint64
	sysGoroutines int
	sysGCPause    uint64

	// Session metrics (UDP-fed from serve process)
	sessNetIn       int64
	sessNetOut      int64
	sessPipeline    string
	sessDeadlocks   int
	sessConnected   bool
	sessLastUpdate  time.Time

	// Dashboard metadata
	boundPort int
	err       error
}

func initialModel() model {
	return model{
		activeTab: tabSummary,
	}
}

// Init returns nil since background goroutines feed data via p.Send().
func (m model) Init() tea.Cmd {
	return nil
}

// Update handles all incoming messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
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
	case systemMsg:
		m.sysUptime = msg.UptimeSeconds
		m.sysMemAlloc = msg.MemoryAllocBytes
		m.sysGoroutines = msg.ActiveGoroutines
		m.sysGCPause = msg.GCPauseNs
	case sessionMsg:
		m.sessNetIn = msg.NetworkBytesRead
		m.sessNetOut = msg.NetworkBytesWritten
		m.sessPipeline = msg.PipelineStage
		m.sessDeadlocks = msg.AporiaDeadlockCount
		m.sessConnected = true
		m.sessLastUpdate = time.Now()
	}

	return m, nil
}

func renderSummary(m model) string {
	b := strings.Builder{}

	// Header in a box
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(1, 4).
		Render(titleStyle.Render("Socratic Thinker Overview"))

	// Connection status
	connStatus := warningStyle.Render("○ Server Disconnected")
	if m.sessConnected && time.Since(m.sessLastUpdate) < 10*time.Second {
		connStatus = successStyle.Render("● Server Connected")
	}
	if m.boundPort > 0 {
		connStatus += metricLabelStyle.Render(fmt.Sprintf("  (udp:%d)", m.boundPort))
	}

	b.WriteString(headerBox + "\n" + connStatus + "\n\n")

	// System Stats Table (self-polled — always live)
	sysRows := [][]string{
		{"Uptime", fmt.Sprintf("%ds", m.sysUptime)},
		{"Memory Allocated", fmt.Sprintf("%.2f MB", float64(m.sysMemAlloc)/1024/1024)},
		{"Goroutines", strconv.Itoa(m.sysGoroutines)},
		{"GC Pause", fmt.Sprintf("%.2fms", float64(m.sysGCPause)/1e6)},
	}
	sysTable := renderStyledTable([]string{"Metric", "Value"}, sysRows)
	sysBox := cardStyle.Render(subTitleStyle.Render("System Stats") + "\n" + sysTable)

	// Session Stats Table (UDP-fed from serve process)
	pipelineStage := m.sessPipeline
	if pipelineStage == "" {
		pipelineStage = metricLabelStyle.Render("No stream")
	}
	sessRows := [][]string{
		{"Net Throughput In", fmt.Sprintf("%d B", m.sessNetIn)},
		{"Net Throughput Out", fmt.Sprintf("%d B", m.sessNetOut)},
		{"Pipeline Stage", pipelineStage},
		{"Aporia Deadlocks", strconv.Itoa(m.sessDeadlocks)},
	}
	sessTable := renderStyledTable([]string{"Metric", "Value"}, sessRows)
	sessBox := cardStyle.Render(subTitleStyle.Render("Session Flow") + "\n" + sessTable)

	// Dynamic layout based on terminal width
	if m.width > 0 && m.width < 100 {
		b.WriteString(lipgloss.JoinVertical(lipgloss.Left, sysBox, sessBox))
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, sysBox, sessBox))
	}

	b.WriteString("\n\n")

	// Recency footer
	if !m.sessLastUpdate.IsZero() {
		ago := int(time.Since(m.sessLastUpdate).Seconds())
		b.WriteString(metricLabelStyle.Render(fmt.Sprintf("Session data last received: %ds ago", ago)))
	} else {
		b.WriteString(metricLabelStyle.Render("Awaiting session data from serve process..."))
	}

	return b.String()
}

// View renders the full TUI.
func (m model) View() string {
	// Build sidebar
	var navLines []string
	navLines = append(navLines, titleStyle.Render("Socratic Dash"))
	navLines = append(navLines, "")

	for i, item := range navItems {
		if i == m.activeTab {
			navLines = append(navLines, activeNavItemStyle.Render("> "+item))
		} else {
			navLines = append(navLines, navItemStyle.Render("  "+item))
		}
	}

	sidebar := sidebarStyle.Render(strings.Join(navLines, "\n"))

	// Build main content
	var content string
	if m.err != nil {
		content = titleStyle.Render("Error") + "\n\n" + fmt.Sprintf("%v", m.err) + "\n\nPress 'q' to quit."
	} else {
		switch m.activeTab {
		case tabSummary:
			content = renderSummary(m)
		case tabQuit:
			content = titleStyle.Render("Quit") + "\n\nPress Enter to exit the dashboard."
		}
	}

	mainView := windowStyle.Render(content)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainView)
}
