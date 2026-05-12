// Package cmd implements the Cobra command tree for the MagicDev MCP server.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/config"
	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/handler"
	"mcp-server-magicdev/internal/integration"
	"mcp-server-magicdev/internal/lifecycle"
	"mcp-server-magicdev/internal/logging"
	"mcp-server-magicdev/internal/sync"
	"mcp-server-magicdev/internal/telemetry"
	gosync "sync"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MagicDev MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("starting magicdev MCP server")

		// LAYER 1: Single-instance enforcement via OS-level file lock.
		// If another instance holds the lock, kill it and take over.
		fileLock, err := lifecycle.AcquireLock()
		if err != nil {
			return fmt.Errorf("single-instance check failed: %w", err)
		}
		defer fileLock.Unlock()

		// Create cancellable context for coordinated graceful shutdown.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Register OS signal handlers (SIGTERM, SIGINT) → cancel context.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			select {
			case sig := <-sigCh:
				slog.Info("received shutdown signal", "signal", sig)
				cancel()
			case <-ctx.Done():
			}
		}()

		// LAYER 2: Parent process watchdog — detect orphaned processes.
		go lifecycle.WatchParent(ctx, cancel)

		// Create MCP server.
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "magicdev",
			Version: Version,
		}, nil)

		// Load YAML config and init fsnotify.
		if err := config.LoadConfig(); err != nil {
			slog.Warn("could not load magicdev.yaml config, proceeding with defaults", "err", err)
		}

		// Reconfigure logger with persistent file output and configured log level.
		logPath, err := logging.Reconfigure(viper.GetString("server.log_level"))
		if err != nil {
			slog.Warn("failed to initialize file logging, continuing with stderr only", "err", err)
		} else {
			slog.Info("file logging initialized", "path", logPath, "level", viper.GetString("server.log_level"))
		}
		defer logging.CloseLogFile()

		// Backward compatibility bindings for Kubernetes / Legacy environments
		viper.BindEnv("confluence.url", "CONFLUENCE_URL")
		viper.BindEnv("jira.url", "JIRA_URL")
		viper.BindEnv("git.username", "GIT_USERNAME")
		viper.BindEnv("git.server_url", "GITLAB_URL")
		viper.BindEnv("git.project_path", "GITLAB_PROJECT_PATH")
		viper.BindEnv("server.db_path", "MAGICDEV_DB_PATH")

		// Apply Runtime Optimizations
		if memLimit := viper.GetString("runtime.gomemlimit"); memLimit != "" {
			memLimit = strings.ToUpper(strings.TrimSpace(memLimit))
			var limitBytes int64
			if strings.HasSuffix(memLimit, "GB") {
				val, err := strconv.ParseInt(strings.TrimSuffix(memLimit, "GB"), 10, 64)
				if err == nil {
					limitBytes = val * 1024 * 1024 * 1024
				}
			} else if strings.HasSuffix(memLimit, "MB") {
				val, err := strconv.ParseInt(strings.TrimSuffix(memLimit, "MB"), 10, 64)
				if err == nil {
					limitBytes = val * 1024 * 1024
				}
			}
			if limitBytes > 0 {
				debug.SetMemoryLimit(limitBytes)
				slog.Info("applied soft memory limit", "bytes", limitBytes, "config", memLimit)
			}
		}

		if maxProcs := viper.GetString("runtime.gomaxprocs"); maxProcs != "" {
			val, err := strconv.Atoi(strings.TrimSpace(maxProcs))
			if err == nil && val > 0 {
				runtime.GOMAXPROCS(val)
				slog.Info("applied max procs limit", "threads", val)
			}
		}

		store, err := db.InitStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		defer store.Close()

		var wg gosync.WaitGroup

		telemetryServer := telemetry.NewServer()
		if telemetryServer != nil {
			telemetryServer.Start()
			defer telemetryServer.Close()
		}

		startTime := time.Now()

		defer func() {
			cancel() // ensure context is canceled so background routines exit
			waitCh := make(chan struct{})
			go func() {
				wg.Wait()
				close(waitCh)
			}()
			select {
			case <-waitCh:
				slog.Debug("all background routines exited gracefully")
			case <-time.After(4 * time.Second):
				slog.Warn("timeout waiting for background routines to exit")
			}
		}()

		// Add metrics reporter: log BuntDB cache entries and latencies every 30 seconds
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					avgReadUs, avgWriteUs, ops := store.GetAndResetLatency()
					slog.Info("BuntDB Telemetry",
						"sessions_count", store.SessionCount(),
						"baselines_count", store.BaselineCount(),
						"chaos_graveyards_count", store.ChaosGraveyardCount(),
						"total_keys", store.DBEntries(),
						"db_size_bytes", store.DBSize(),
						"interval_ops", ops,
						"avg_read_us", avgReadUs,
						"avg_write_us", avgWriteUs,
					)
				}
			}
		}()

		// UDP Telemetry Broadcaster (Hot State)
		if telemetryServer != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(telemetry.EmissionInterval)
				defer ticker.Stop()
				var memStats runtime.MemStats
				
				stages := []string{
					"evaluate_idea", "ingest_standards", "clarify_requirements",
					"critique_design", "finalize_requirements", "blueprint_implementation",
					"generate_documents", "complete_design",
				}

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						runtime.ReadMemStats(&memStats)
						
						payload := telemetry.MetricPayload{
							NumCPU:       runtime.NumCPU(),
							NumGoroutine: runtime.NumGoroutine(),
							MemAlloc:     memStats.Alloc,
							NextGC:       memStats.NextGC,
							GOMemLimit:   viper.GetString("runtime.gomemlimit"),
							Uptime:       time.Since(startTime).Round(time.Second).String(),
						}

						if payload.GOMemLimit == "" {
							payload.GOMemLimit = "Unknown"
						}

						// Hot State: Active Pipeline Session
						sessions, _ := store.ListSessions()
						if len(sessions) > 0 {
							latest := sessions[len(sessions)-1]
							isComplete := false
							if s, ok := latest.StepStatus["complete_design"]; ok && (s == "DONE" || s == "COMPLETED" || s == "FAILED") {
								isComplete = true
							}

							for _, stage := range stages {
								status := "PENDING"
								latency := "-"
								tokenDelta := "-"
								sessionDataStr := "-"

								if isComplete {
									status = "IDLE"
								}

								if s, ok := latest.StepStatus[stage]; ok && s != "" {
									status = s
								} else if latest.CurrentStep == stage && !isComplete {
									status = "ACTIVE"
								}

								if t, ok := latest.StepTimings[stage]; ok {
									if t.DurationMs > 0 {
										latency = fmt.Sprintf("%ds", t.DurationMs/1000)
									} else if t.StartedAt != "" {
										if startedAt, err := time.Parse(time.RFC3339, t.StartedAt); err == nil {
											latency = fmt.Sprintf("%ds", int(time.Since(startedAt).Seconds()))
										}
									}
								}
								
								if toks, ok := latest.StepTokens[stage]; ok {
									tokenDelta = fmt.Sprintf("+%d", toks)
								}
								if bytes, ok := latest.StepDataBytes[stage]; ok {
									const unit = 1024
									b := uint64(bytes)
									if b < unit {
										sessionDataStr = fmt.Sprintf("%d B", b)
									} else {
										div, exp := uint64(unit), 0
										for n := b / unit; n >= unit; n /= unit {
											div *= unit
											exp++
										}
										sessionDataStr = fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
									}
								}

								payload.PipelineStages = append(payload.PipelineStages, telemetry.StageTelemetry{
									Name:           stage,
									Status:         status,
									Latency:        latency,
									TokenDelta:     tokenDelta,
									SessionDataStr: sessionDataStr,
								})
							}
						}

						telemetryServer.Broadcast(payload)
					}
				}
			}()
		}

		// Auto-provisioning logic via environment variables
		provisionVault(store, "gitlab", "GITLAB_TOKEN", "GITLAB_PERSONAL_ACCESS_TOKEN", "GITLAB_USER_TOKEN")
		provisionVault(store, "jira", "JIRA_USER_TOKEN", "JIRA_TOKEN", "JIRA_API_TOKEN")
		provisionVault(store, "confluence", "CONFLUENCE_USER_TOKEN", "CONFLUENCE_TOKEN", "CONFLUENCE_API_TOKEN")

		// Security & Environment Parameters validation hook
		checkVaultSecret(store, "confluence")
		checkVaultSecret(store, "jira")
		checkVaultSecret(store, "gitlab")

		// Launch the background baseline standards sync priority cascade
		wg.Add(1)
		go func() {
			defer wg.Done()
			sync.SyncBaselines(store)
		}()

		// Start live filesystem watcher for local standards — automatically
		// updates BuntDB cache when .md files change on disk.
		sync.StartStandardsWatcher(ctx, &wg, store)

		// Intelligence Engine (LLM) initialization and health check closure
		checkLLMHealth := func() {
			if client, err := integration.NewLLMClient(store); err != nil {
				slog.Info("Intelligence Engine (LLM) feature disabled or unconfigured", "reason", err)
			} else {
				// Retry health check up to 3 times to handle cold starts and transient failures
				const maxRetries = 3
				var healthy bool

				for attempt := 1; attempt <= maxRetries; attempt++ {
					pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					_, pingErr := client.GenerateContent(pingCtx, "ping")
					cancel()

					if pingErr == nil {
						slog.Info("Intelligence Engine (LLM) feature enabled and healthy",
							"model", viper.GetString("llm.model"),
							"attempt", attempt,
						)
						healthy = true
						break
					}

					slog.Warn("Intelligence Engine (LLM) health check failed",
						"attempt", attempt,
						"max_retries", maxRetries,
						"model", viper.GetString("llm.model"),
						"error", pingErr,
					)
					if attempt < maxRetries {
						time.Sleep(time.Duration(1<<attempt) * time.Second) // 2s, 4s
					}
				}

				if !healthy {
					slog.Warn("Intelligence Engine (LLM) is configured but failing after all retries",
						"model", viper.GetString("llm.model"),
					)
				}
			}
		}

		// Execute health check at startup and register it for hot-reload
		checkLLMHealth()
		config.OnConfigReload = append(config.OnConfigReload, checkLLMHealth)

		handler.RegisterTools(s, store)
		handler.RegisterPrompts(s)

		slog.Info("MCP server ready", "version", Version)

		// Run MCP server with cancellable context.
		runErr := s.Run(ctx, &mcp.StdioTransport{})

		// LAYER 3: Shutdown deadline — force exit if cleanup hangs.
		lifecycle.ShutdownDeadline(5 * time.Second)

		slog.Info("MCP server shutting down gracefully")
		return runErr
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func provisionVault(store *db.Store, service string, envKeys ...string) {
	for _, envKey := range envKeys {
		if envVal := os.Getenv(envKey); envVal != "" {
			if err := store.SetSecret(service, envVal); err != nil {
				slog.Error("failed to auto-provision vault", "service", service, "err", err)
			} else {
				slog.Info("auto-provisioned vault secret from environment", "service", service, "env", envKey)
				os.Unsetenv(envKey)
			}
			return
		}
	}
}

func checkVaultSecret(store *db.Store, service string) {
	val, err := store.GetSecret(service)
	if err != nil || val == "" {
		slog.Warn("missing secret in vault", "service", service, "action", "run 'mcp-server-magicdev token update' to configure")
	}
}
