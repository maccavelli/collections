package cmd

import (
	"bufio"
	"strings"
	"testing"
)

func TestPromptInput(t *testing.T) {
	input := "test-input\n"
	reader := bufio.NewReader(strings.NewReader(input))
	
	result, err := promptInput(reader, "Test Label")
	if err != nil {
		t.Fatalf("Failed to prompt input: %v", err)
	}
	
	if result != "test-input" {
		t.Errorf("Expected 'test-input', got '%s'", result)
	}
}

func TestPromptInputError(t *testing.T) {
	// A reader that returns EOF immediately
	reader := bufio.NewReader(strings.NewReader(""))
	
	_, err := promptInput(reader, "Test Label")
	if err == nil {
		t.Error("Expected error on empty input reader")
	}
}
