package config

import (
	"time"
)

const (
	// Project Identity
	Name     = "mcp-server-duckduckgo"
	Platform = "DuckDuckGo"

	// Network Context
	DefaultTimeout = 15 * time.Second
	MaxBodyBytes   = 10 * 1024 * 1024 // 10 MB limit for safety
	UserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

	// Scraping Hardening
	MaxSnippetLength = 1000
	VQDCacheTTL      = 5 * time.Minute
	VQDCacheLimit    = 500 // Max unique queries in cache

	// Logging: get_internal_logs buffer limits
	LogBufferLimit  = 1024 * 1024 // 1MB max log buffer
	LogTrimTarget   = 512 * 1024  // 512KB trim target
	DefaultLogLines = 25          // Default lines returned by get_internal_logs
)
