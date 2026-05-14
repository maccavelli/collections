// Package telemetry provides functionality for the telemetry subsystem.
package telemetry

import (
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"sync"
)

// Server handles the UDP broadcast of telemetry data to the dashboard.
type Server struct {
	conn            *net.UDPConn
	dashboardAddr   *net.UDPAddr
	dashboardAddrMu sync.Mutex
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

	return &Server{conn: conn}
}

// Start begins listening for dashboard pings to register the client address.
func (s *Server) Start() {
	if s == nil || s.conn == nil {
		return
	}

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

// Broadcast sends the MetricPayload to the connected dashboard.
func (s *Server) Broadcast(payload MetricPayload) {
	if s == nil || s.conn == nil {
		return
	}

	s.dashboardAddrMu.Lock()
	target := s.dashboardAddr
	s.dashboardAddrMu.Unlock()

	if target == nil {
		return
	}

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
	_ = s.conn.Close()
}
