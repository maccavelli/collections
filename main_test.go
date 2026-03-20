package main

import (
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/state"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestLogBuffer_Trimming(t *testing.T) {
	lb := &LogBuffer{}
	
	// Basic Write/String
	msg := "test log message\n"
	n, err := lb.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(msg) {
		t.Errorf("want %d bytes, got %d", len(msg), n)
	}
	if lb.String() != msg {
		t.Errorf("want '%s', got '%s'", msg, lb.String())
	}

	// Trimming logic
	data := make([]byte, 1024*1024+100)
	for i := range data {
		data[i] = 'a'
		if i%100 == 0 {
			data[i] = '\n'
		}
	}
	_, _ = lb.Write(data)
	if lb.buf.Len() > logBufferLimit {
		t.Errorf("buffer length %d exceeds limit after trimming", lb.buf.Len())
	}
}

func TestLogBuffer_TrimmingNoNewline(t *testing.T) {
	lb := &LogBuffer{}
	
	// Write data without newlines in the trim range.
	data := make([]byte, 1024*1024+100)
	for i := range data {
		data[i] = 'a'
	}
	
	_, _ = lb.Write(data)
	if lb.buf.Len() > logBufferLimit {
		t.Errorf("buffer length %d exceeds limit after trimming", lb.buf.Len())
	}
}

func TestVersionReporting(t *testing.T) {
	printVersion()
}

func TestToolRegistrations(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0")
	mgr := state.NewManager(".")
	ports := engine.NewEngine(".")
	
	registerDiscoveryTools(s, mgr, ports)
	registerDesignTools(s, ports)
	registerDecisionTools(s, ports)
	
	tools := s.ListTools()
	if len(tools) == 0 {
		t.Error("expected tools to be registered")
	}
}
