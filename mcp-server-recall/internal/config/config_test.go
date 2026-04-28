package config

import (
	"testing"
)

func TestConfig_Defaults(t *testing.T) {
	c := New("test-server")

	_ = c.DedupThreshold()
	b := c.BatchSettings()
	if b.MaxBatchSize <= 0 {
		t.Errorf("expected valid batch size")
	}

	_ = c.ExportDir()
	_ = c.EncryptionKey()
	_ = c.HarvestDisableDrift()
}
