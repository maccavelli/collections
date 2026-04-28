package config

import (
	"testing"
)

func TestConfigConstantsNative(t *testing.T) {
	if Name == "" {
		t.Error("unexpected empty Name constant")
	}
	if LogBufferLimit == 0 {
		t.Error("unexpected empty limit")
	}
	if LogTrimTarget == 0 {
		t.Error("unexpected empty trim target")
	}
	if DefaultLogLines == 0 {
		t.Error("unexpected default line constraint")
	}
}
