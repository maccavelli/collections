package logging

import (
	"strings"
	"testing"
)

func TestLogBufferRedaction(t *testing.T) {
	lb := &LogBuffer{}
	
	input := []byte("User logged in with token_abcdef123 and secret_xyz456 successfully.")
	_, err := lb.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	
	output := lb.String()
	if strings.Contains(output, "token_abcdef123") || strings.Contains(output, "secret_xyz456") {
		t.Errorf("Secrets were not redacted: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Expected [REDACTED] in output, got: %s", output)
	}
}

func TestLogBufferLimit(t *testing.T) {
	lb := &LogBuffer{}
	
	// Create a payload that is slightly larger than the limit
	// LogBufferLimit is 2MB, so we'll write 2.1MB
	payloadSize := 2*1024*1024 + 100*1024 
	payload := make([]byte, payloadSize)
	for i := range payload {
		if i%100 == 0 {
			payload[i] = '\n'
		} else {
			payload[i] = 'A'
		}
	}
	
	_, err := lb.Write(payload)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	
	// Ensure the buffer was trimmed to approx LogTrimTarget (1MB)
	outputSize := lb.buf.Len()
	if outputSize > 11*1024*1024/10 || outputSize < 9*1024*1024/10 {
		t.Errorf("Expected buffer to be trimmed to ~1MB, got %d bytes", outputSize)
	}
}

func TestTailLines(t *testing.T) {
	logOutput := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	
	tail := TailLines(logOutput, 3)
	expected := "Line 3\nLine 4\nLine 5"
	if tail != expected {
		t.Errorf("Expected %q, got %q", expected, tail)
	}
	
	tailAll := TailLines(logOutput, 10)
	expectedAll := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	if tailAll != expectedAll {
		t.Errorf("Expected %q, got %q", expectedAll, tailAll)
	}
	
	emptyTail := TailLines("", 5)
	if emptyTail != "" {
		t.Errorf("Expected empty string, got %q", emptyTail)
	}
}
