package telemetry

import (
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	// TelemetryPorts are the UDP ports used for dashboard telemetry (serve listens, dashboard connects).
	TelemetryPorts = []int{49801, 49802, 49803, 49804, 49805}
	// EmissionInterval controls how frequently the serve process pushes metrics to the dashboard.
	EmissionInterval = 500 * time.Millisecond
)

// MetricPayload contains the Hot State telemetry data sent over UDP.
type MetricPayload struct {
	// System Metrics
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemAlloc     uint64 `json:"mem_alloc"`
	NextGC       uint64 `json:"next_gc"`
	GOMemLimit   string `json:"go_mem_limit"`
	Uptime       string `json:"uptime"`

	// 8-Stage Pipeline Telemetry & Session Flow
	PipelineStages []StageTelemetry `json:"pipeline_stages"`
}

// StageTelemetry represents the real-time status of a single pipeline stage.
type StageTelemetry struct {
	Name           string `json:"name"`
	Status         string `json:"status"` // PENDING, ACTIVE, DONE, FAILED, IDLE
	Latency        string `json:"latency"`
	TokenDelta     string `json:"token_delta"`
	SessionDataStr string `json:"session_data_str"`
}

// Server handles the UDP broadcast of telemetry data.
type Server struct {
	conn            *net.UDPConn
	dashboardAddr   *net.UDPAddr
	dashboardAddrMu sync.Mutex
	stopCh          chan struct{}
}

// NewServer initializes the UDP listener on the first available port.
func NewServer() *Server {
	var conn *net.UDPConn
	for _, port := range TelemetryPorts {
		addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
		c, err := net.ListenUDP("udp", addr)
		if err == nil {
			conn = c
			slog.Info("telemetry udp listener bound", "port", port)
			break
		}
		slog.Warn("telemetry port unavailable", "port", port, "error", err)
	}

	if conn == nil {
		slog.Warn("all telemetry ports exhausted; starting without dashboard emission")
		return nil
	}

	return &Server{
		conn:   conn,
		stopCh: make(chan struct{}),
	}
}

// Start begins listening for dashboard pings to register the client address.
func (s *Server) Start() {
	if s == nil || s.conn == nil {
		return
	}

	// Goroutine to receive pings from the dashboard
	go func() {
		buf := make([]byte, 64)
		for {
			_, remoteAddr, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				if strings.Contains(err.Error(), "use of closed") {
					return
				}
				continue
			}
			s.dashboardAddrMu.Lock()
			s.dashboardAddr = remoteAddr
			s.dashboardAddrMu.Unlock()
		}
	}()
}

// Broadcast sends the MetricPayload to the connected dashboard if debouncing allows it.
// It is non-blocking.
func (s *Server) Broadcast(payload MetricPayload) {
	if s == nil || s.conn == nil {
		return
	}

	s.dashboardAddrMu.Lock()
	target := s.dashboardAddr
	s.dashboardAddrMu.Unlock()

	if target == nil {
		return // No dashboard connected yet
	}

	// In this implementation, the debouncing should be handled by the caller or by a ticker
	// However, if we want Broadcast itself to debounce:
	// We'll implement a simple non-blocking write. If the network is choked, we don't block.
	data, err := json.Marshal(payload)
	if err == nil {
		_, _ = s.conn.WriteToUDP(data, target)
	}
}

// Close gracefully shuts down the UDP listener.
func (s *Server) Close() {
	if s == nil || s.conn == nil {
		return
	}
	close(s.stopCh)
	_ = s.conn.Close()
}
