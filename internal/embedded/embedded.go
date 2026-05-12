package embedded

import "embed"

// FS is the embedded filesystem containing static assets and templates.
//
//go:embed templates/* standards/*
var FS embed.FS
