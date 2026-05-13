package telemetry_test

import (
	"strings"
	"testing"

	"mcp-server-socratic-thinker/internal/telemetry"
)

func TestRingBuffer_Write(t *testing.T) {
	rb := telemetry.NewRingBuffer(3)

	// Write basic lines
	msg := "line1\nline2\nline3\n"
	n, err := rb.Write([]byte(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(msg), n)
	}

	out := rb.String()
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line3") {
		t.Errorf("output missing expected lines: %s", out)
	}

	// Write more to overflow buffer
	rb.Write([]byte("line4\n"))
	out = rb.String()
	if strings.Contains(out, "line1") {
		t.Errorf("expected line1 to be evicted, output: %s", out)
	}
	if !strings.Contains(out, "line4") {
		t.Errorf("expected line4 to be present, output: %s", out)
	}
}

func TestRingBuffer_AddLog(t *testing.T) {
	rb := telemetry.NewRingBuffer(2)

	rb.AddLog("INFO", "msg1")
	rb.AddLog("WARN", "msg2")

	out := rb.String()
	if !strings.Contains(out, "[INFO] msg1") || !strings.Contains(out, "[WARN] msg2") {
		t.Errorf("unexpected output: %s", out)
	}

	rb.AddLog("ERROR", "msg3")
	out = rb.String()
	if strings.Contains(out, "msg1") {
		t.Errorf("expected msg1 to be evicted, output: %s", out)
	}
	if !strings.Contains(out, "[ERROR] msg3") {
		t.Errorf("expected msg3 to be present, output: %s", out)
	}
}

func TestRingBuffer_Concurrency(t *testing.T) {
	rb := telemetry.NewRingBuffer(100)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 10; j++ {
				rb.AddLog("INFO", "concurrent log")
				rb.Write([]byte("concurrent write\n"))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	out := rb.String()
	// Just ensure it doesn't panic and has some content
	if len(out) == 0 {
		t.Errorf("expected some output from concurrent writes")
	}
}
