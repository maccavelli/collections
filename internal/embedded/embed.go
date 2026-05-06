// Package embedded provides functionality for the embedded subsystem.
package embedded

import "embed"

// BaselineFS holds the embedded baseline markdown standards for air-gapped environments.
//
//go:embed standards/dotnet/*.md standards/node/*.md
var BaselineFS embed.FS
