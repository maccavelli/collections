package util

import (
	"testing"
)

func TestLoggingFallback_Magic(t *testing.T) {
	cleanup := SetupStandardLogging("magicskills", nil)
	cleanup()
}
