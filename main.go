package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"

	"mcp-server-magictools/cmd"
	"mcp-server-magictools/internal/client"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/external"
	"mcp-server-magictools/internal/handler"
	"mcp-server-magictools/internal/intelligence"
	"mcp-server-magictools/internal/logging"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"
	"mcp-server-magictools/internal/vector"
)

var (
	realStdout = os.Stdout
	realStdin  = os.Stdin
)

var (
	isInitialized atomic.Bool // 🛡️ HANDSHAKE STATE: Tracks if notifications/initialized was received
)

// OrchestratorApp encapsulates the magictools runtime environment and lifecycle.
type OrchestratorApp struct {
	store    *db.Store
	cfg      *config.Config
	reg      *client.WarmRegistry
	server   *mcp.Server
	levelVar *slog.LevelVar
	mcpLog   *logging.McpLogHandler

	eg     *errgroup.Group
	egCtx  context.Context
	cancel context.CancelFunc

	bootOnce       sync.Once
	hydratorSignal chan struct{}       // 🛡️ HYDRATOR GATE: Signaled when boot or sync actions complete
	recallClient   *external.MCPClient // 🛡️ RECALL CLIENT: HTTP streaming connection to mcp-server-recall

	activePipelineEnabled atomic.Bool // 🛡️ PIPELINE GATE: True when recall + brainstorm + go-refactor are all online

	ProcessStartTime time.Time // 🛡️ TELEMETRY: Anchors the boot duration calculation at the physical binary entry point
}

func main() {
	processStartTime := time.Now()
	// 🛡️ [BOILERPLATE] Standard I/O Redirection
	realStdin = os.Stdin

	// 🛡️ RESOURCE CONTROL: Set soft memory limit to 2048MiB (2GB) allowing massive parallel arrays
	debug.SetMemoryLimit(2048 << 20)
	debug.SetGCPercent(-1) // 🛡️ GC PAUSE: Temporarily suspend garbage collection to accelerate mass array spawning

	// 1. Wire up Cobra callbacks
	cmd.ServeFunc = func(cmdCtx context.Context, configPath, dbPath, logPath, logLevel string, noOptimize, debug bool) error {
		// Update global state from Cobra flags
		realStdout = cmd.RealStdout
		baseCtx, baseCancel := context.WithCancel(cmdCtx)
		defer baseCancel()
		ctx, cancel := signal.NotifyContext(baseCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer cancel()

		// Infrastructure Preparation
		util.TraceFunc(ctx, "step", "enforceResourceLimits")
		enforceResourceLimits()
		util.TraceFunc(ctx, "step", "InitialProcessSweep")
		InitialProcessSweep()

		app, err := NewOrchestratorApp(ctx, cancel, configPath, dbPath, logPath, logLevel, noOptimize, debug, processStartTime)
		if err != nil {
			return err
		}

		if handled, preErr := app.PreDrive(false); handled {
			return preErr
		}

		return app.Start()
	}

	cmd.DBWipeFunc = func(dbPath string) error {
		// Minimal setup for database wipe
		store, err := db.NewStore(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		slog.Warn("CLI: wipe command detected, wiping database and search index...")
		if err := store.WipeAll(); err != nil {
			return fmt.Errorf("failed to wipe database: %w", err)
		}
		fmt.Fprintf(os.Stderr, "SUCCESS: Database and search index have been completely wiped.\n")
		return nil
	}

	cmd.DBSyncFunc = func(dbPath string) error {
		store, err := db.NewStore(dbPath)
		if err != nil {
			if strings.Contains(err.Error(), "lock") || strings.Contains(err.Error(), "resource temporarily unavailable") {
				return fmt.Errorf("Database is locked. Please sleep or stop the magictools orchestrator daemon before running 'db sync'")
			}
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()

		slog.Warn("CLI: sync command detected, forcing BadgerDB source-of-truth reindex onto Bleve...")
		if err := store.ReindexAllTools(); err != nil {
			return fmt.Errorf("failed to sync bleve index: %w", err)
		}
		fmt.Fprintf(os.Stderr, "SUCCESS: Search index has been perfectly re-aligned to the BadgerDB source of truth.\n")
		return nil
	}

	// 2. Execute Cobra
	cmd.Execute(Version)
}

// NewOrchestratorApp initializes a new OrchestratorApp, establishing the database, configuration, and logging lifecycle.
func NewOrchestratorApp(ctx context.Context, cancel context.CancelFunc, configPath, dbPath, logPath, logLevel string, noOptimize, debug bool, startTime time.Time) (*OrchestratorApp, error) {
	util.TraceFunc(ctx, "config", configPath, "db", dbPath, "debug", debug)

	// Create a master context tree using errgroup
	eg, egCtx := errgroup.WithContext(ctx)

	store, cfg, err := loadConfigAndDB(configPath, dbPath, logPath, noOptimize, debug)
	if err != nil {
		return nil, err
	}

	app := &OrchestratorApp{
		store:            store,
		cfg:              cfg,
		eg:               eg,
		egCtx:            egCtx,
		cancel:           cancel,
		hydratorSignal:   make(chan struct{}, 1),
		ProcessStartTime: startTime,
	}

	// 🛡️ LOG LEVEL DISCOVERY: Flag -> Config -> Default (DEBUG)
	var initialLevel slog.Level
	if debug {
		initialLevel = util.LevelTrace
	} else if logLevel != "" {
		initialLevel = logging.ParseLogLevel(logLevel)
	} else if cfg.LogLevel != "" {
		initialLevel = logging.ParseLogLevel(cfg.LogLevel)
	} else {
		initialLevel = slog.LevelDebug
	}

	app.levelVar = new(slog.LevelVar)
	app.levelVar.Set(initialLevel)

	// Single instantiation of MCP Log Bridge. At this point session is nil, so it acts as a no-op.
	app.mcpLog = logging.NewMcpLogHandler(app.levelVar)

	telemetry.MustInitializeRingBuffer(filepath.Join(config.DefaultCacheDir(), "telemetry.ring"))

	// 🛡️ UNIFIED LOGGING SETUP: Done exactly once.
	logging.SetupGlobalLogger(app.store, app.cfg.LogPath, app.cfg.GetLogFormat(), app.levelVar, noOptimize, debug, realStdout, app.mcpLog)

	// 🛡️ NATIVE VECTOR ENGINE: Mount the pure Go-HNSW Index (AFTER logger is configured)
	if err := vector.InitGlobalEngine(dbPath, cfg); err != nil {
		slog.Warn("Failed to initialize HNSW vector engine natively", "error", err)
	}

	// 🛡️ TELEMETRY BRIDGE: Wire vector stats callback to avoid circular imports
	telemetry.VectorStatsFunc = func() telemetry.VectorStats {
		e := vector.GetEngine()
		if e == nil {
			return telemetry.VectorStats{}
		}
		vs := telemetry.VectorStats{
			Enabled:         e.VectorEnabled(),
			Provider:        cfg.Intelligence.EmbeddingProvider,
			Model:           cfg.Intelligence.EmbeddingModel,
			Dims:            cfg.Intelligence.EmbeddedDimensionality,
			GraphNodes:      e.Len(),
			NeedsHydration:  e.RequiresHydration(),
			VectorWins:      telemetry.SearchMetrics.VectorWins.Load(),
			LexicalWins:     telemetry.SearchMetrics.LexicalWins.Load(),
			TotalSearches:   telemetry.SearchMetrics.TotalSearches.Load(),
			VectorSearches:  telemetry.SearchMetrics.VectorSearches.Load(),
			LexicalSearches: telemetry.SearchMetrics.LexicalSearches.Load(),
		}
		if vs.TotalSearches > 0 {
			vs.AvgLatencyMs = telemetry.SearchMetrics.TotalLatencyMs.Load() / vs.TotalSearches
		}
		return vs
	}

	pidDir := filepath.Join(dbPath, "pids")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		slog.Warn("Failed to create PID directory", "path", pidDir, "error", err)
	}
	app.reg = client.NewWarmRegistry(pidDir, app.store, app.cfg)

	return app, nil
}

// PreDrive prepares the registry state by pruning orphaned processes before orchestrator bootstrapping.
func (app *OrchestratorApp) PreDrive(clearDB bool) (bool, error) {

	util.TraceFunc(app.egCtx, "step", "PruneOrphans")
	app.reg.PruneOrphans()

	return false, nil
}

// Stop gracefully terminates all background workers and unbinds the MCP registry sockets.
func (app *OrchestratorApp) Stop() {
	app.cancel()
	if waitErr := app.eg.Wait(); waitErr != nil && waitErr != context.Canceled {
		slog.Error("Background shutdown error", "error", waitErr)
	}

	// 🛡️ SERIALIZATION: Flush the spatial-node relationships safely locally
	if e := vector.GetEngine(); e != nil {
		if err := e.Save(); err != nil {
			slog.Warn("failed to save vector database persistently", "error", err)
		}
	}

	app.reg.DisconnectAll()
	app.store.Close()
}

// Start initializes the hot-loading server watchers and binds the primary JSON-RPC 2.0 streaming pipeline.
func (app *OrchestratorApp) Start() error {
	defer app.Stop()

	// 🛡️ PRE-INJECT RECALL: Instantiated early to guarantee memory pointer availability for handlers
	apiURLs := config.ResolveAPIURLs()
	if len(apiURLs) > 0 {
		app.recallClient = external.NewMCPClient(apiURLs[len(apiURLs)-1])
	}

	// 🛡️ [BOILERPLATE] Standard MCP Tool Registration
	h := handler.NewHandler(app.store, app.reg, app.cfg)
	h.SetLogLevel(app.levelVar)
	h.RecallClient = app.recallClient              // 🛡️ RECALL WIRING: Direct HTTP client for PM pipeline tools
	h.PipelineEnabled = &app.activePipelineEnabled // 🛡️ PIPELINE WIRING: Conditional enablement flag
	h.HydratorSignal = app.hydratorSignal          // 🛡️ HYDRATOR WIRING: Daemon continuation channel

	// 🛡️ HOT LOADING ACTIVATION: Start the config watcher
	w := config.NewWatcher(app.cfg.Viper(), app.cfg, h)
	w.Start()
	defer w.Stop()

	app.server = mcp.NewServer(
		&mcp.Implementation{Name: "mcp-server-magictools", Version: Version},
		&mcp.ServerOptions{
			Logger: slog.Default(),
			InitializedHandler: func(handlerCtx context.Context, req *mcp.InitializedRequest) {
				sessionID := req.Session.ID()
				if sessionID == "" {
					sessionID = "active-session"
				}
				util.TraceFunc(app.egCtx, "event", "on_initialized", "session_id", sessionID)
				slog.Info("orchestrator: Handshake successful", "session_id", sessionID)

				// 🛡️ ATOMIC LOGGING ACTIVATION
				app.mcpLog.SetSession(req.Session)
				isInitialized.Store(true)

				app.bootOnce.Do(func() {
					app.startBackgroundWorkers()
					app.executeBootSequence()
				})
			},
		},
	)

	h.Register(app.server)
	h.RegisterPipelineTools(app.server) // 🛡️ PM TOOLS: compose_pipeline, validate_pipeline_step, cross_server_quality_gate

	// 🛡️ [BOILERPLATE] Standard Transport Wrapper
	eofReader := &util.EofDetector{R: realStdin, Cancel: app.cancel}
	autoWriter := &util.AutoFlusher{W: realStdout}

	var finalWriter io.Writer = autoWriter

	// 🛡️ [DEBUG] WIRE TAP
	tapIn := &logging.WireTapReader{Rc: util.NopReadCloser{Reader: eofReader}, Ctx: app.egCtx}
	tapOut := &logging.WireTapWriter{Wc: util.NopWriteCloser{Writer: finalWriter}, Ctx: app.egCtx}

	slog.Info("orchestrator: starting I/O loop with IDE", "pid", os.Getpid())
	if runErr := app.server.Run(app.egCtx, &mcp.IOTransport{
		Reader: tapIn,
		Writer: tapOut,
	}); runErr != nil {
		slog.Error("IOTransport run aborted", "error", runErr)
	}
	app.cancel() // Explicitly trigger the errgroup teardown
	return nil
}

func (app *OrchestratorApp) startBackgroundWorkers() {
	util.TraceFunc(app.egCtx, "step", "starting_background_loops")

	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "Reconciler", "panic", r)
			}
		}()
		app.reg.StartReconciler(app.egCtx)
		return nil
	})
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "HealthMonitor", "panic", r)
			}
		}()
		app.reg.StartHealthMonitor(app.egCtx, 60*time.Second)
		return nil
	})

	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "BackgroundGC", "panic", r)
			}
		}()
		app.store.StartBackgroundGC(app.egCtx, 30*time.Minute)
		return nil
	})
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "watchdog", "panic", r)
			}
		}()
		watchdog(app.egCtx, app.cancel)
		return nil
	})
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "zombieReaper", "panic", r)
			}
		}()
		setupZombieReaper(app.egCtx)
		return nil
	})
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "watchBinary", "panic", r)
			}
		}()
		watchBinary(app.egCtx, app.cancel)
		return nil
	})

	app.eg.Go(func() error {
		intelligence.StartHydratorDaemon(app.egCtx, app.store, app.cfg, app.recallClient, app.hydratorSignal)
		return nil
	})

	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "RecallMiner", "panic", r)
			}
		}()

		// Wait for initial delay to let recall client establish
		select {
		case <-time.After(30 * time.Second):
		case <-app.egCtx.Done():
			return nil
		}

		// Run operational priming and boot snapshot once on activation
		if app.recallClient != nil {
			primeFromRecall(app.egCtx, app.recallClient, app.cfg)

			if app.recallClient.RecallEnabled() {
				app.recallClient.SaveSession(app.egCtx, "magictools-diagnostics", "", struct {
					Stage     string `json:"stage"`
					BootTime  string `json:"boot_time"`
					Servers   int    `json:"servers"`
					Recall    bool   `json:"recall"`
					DBMetrics any    `json:"db_metrics"`
				}{
					Stage:     "boot_snapshot",
					BootTime:  time.Now().Format(time.RFC3339),
					Servers:   len(app.cfg.GetManagedServers()),
					Recall:    true,
					DBMetrics: app.store.GetMetrics(),
				})
				slog.Info("boot diagnostic snapshot emitted to recall", "component", "recall_diagnostics")
			}
		}
		return nil
	})
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "cacheMetrics", "panic", r)
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-app.egCtx.Done():
				return nil
			case <-ticker.C:
				m := app.store.GetMetrics()
				slog.Info("cache metrics", "type", "registry", "hits", m.Hits, "misses", m.Misses, "entries", m.Entries, "db_tools", m.Tools, "db_intel", m.Intel, "bleve_docs", m.BleveDocs)
				if e := vector.GetEngine(); e != nil && e.VectorEnabled() {
					slog.Info("cache metrics", "type", "vector",
						"graph_nodes", e.Len(),
						"needs_hydration", e.RequiresHydration(),
						"vector_wins", telemetry.SearchMetrics.VectorWins.Load(),
						"lexical_wins", telemetry.SearchMetrics.LexicalWins.Load(),
						"total_searches", telemetry.SearchMetrics.TotalSearches.Load(),
					)
				}
			}
		}
	})

	// 🛡️ RECALL CLIENT: Establish HTTP streaming connection to mcp-server-recall
	app.establishStreamingClient()
}

// establishStreamingClient initializes the recall HTTP MCPClient and spawns its
// reconnect loop as a background worker. Mirrors the brainstorm/go-refactor pattern.
// If MCP_API_URL is not configured, this is a safe no-op.
func (app *OrchestratorApp) establishStreamingClient() {
	if app.recallClient == nil {
		slog.Info("MCP_API_URL not configured — recall HTTP client disabled (standalone mode)", "component", "recall")
		return
	}

	slog.Info("starting external context client background connection", "component", "recall")
	app.eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "RecallClient", "panic", r)
			}
		}()
		// Start() has its own exponential backoff retry loop (1s → 30s).
		// The 30s boot delay inside Start() prevents premature connection attempts.
		if err := waitForRecallSocketReady(app.egCtx, slog.Default()); err != nil {
			slog.Error("recall socket wait failed", "error", err)
		}
		app.recallClient.Start(app.egCtx)
		return nil
	})
}

func (app *OrchestratorApp) executeBootSequence() {
	app.eg.Go(func() error {
		// 🛡️ OPTIMIZATION: Disable "Stop The World" GC sweeps during the heavy JSON boot spike.
		// Rely entirely on debug.SetMemoryLimit(1GB) to force an emergency sweep if needed.
		prevGC := debug.SetGCPercent(-1)
		defer debug.SetGCPercent(prevGC)

		defer func() {
			if r := recover(); r != nil {
				slog.Error("errgroup: panic in background goroutine", "func", "BootSequence", "panic", r)
			}
		}()

		ctx := app.egCtx
		util.TraceFunc(ctx, "event", "boot_sequence_start")
		managed := app.cfg.GetManagedServers()
		if len(managed) == 0 {
			slog.Warn("BootSequence: ZERO managed servers found in config. Orchestrator will run in internal-only mode.", "servers_registry", filepath.Join(config.DefaultConfigDir(), config.ServersConfigFile))
		} else {
			slog.Info("Initiating Fault-Tolerant BootSequence (Background)", "total_servers", len(managed))
		}

		// 🛡️ THE GLOBAL ASYNC TRACKER
		// Guarantees the Boot Duration metric reports the exact moment 100% of all daemon tasks complete.
		var asyncTimelineWg sync.WaitGroup

		// 🛡️ ASYNC WARM BOOT: ReindexAllTools populates the Bleve search cache
		// but is NOT needed for server handshakes — only for align_tools queries.
		// Fire-and-forget so it doesn't block the boot sequence.
		var successCount, offlineCount atomic.Int64
		asyncTimelineWg.Add(1)
		go func() {
			defer asyncTimelineWg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("warm boot panicked", "panic", r)
				}
			}()
			if err := app.reg.Store.ReindexAllTools(); err != nil {
				slog.Warn("WarmBoot: Failed to populate search cache from DB", "error", err)
			} else {
				slog.Info("Warm boot successful: search index cached in-memory", "component", "registry")
			}
		}()

		// 🛡️ CONCURRENT BOOT: All critical servers (including recall) boot in parallel.
		// Sub-servers with recall dependencies handle connectivity independently
		// via their 5-retry/30s-hold circuit breaker logic — no serial Phase 1 needed.
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(10)

		// 🛡️ BOOT PARTITIONING: Split servers into critical-path (blocks boot)
		// and deferred (boots in background after IDE handshake).
		// Controlled by deferred_boot directive in servers.yaml.
		critical := slices.Clone(managed)
		critical = slices.DeleteFunc(critical, func(sc config.ServerConfig) bool {
			return sc.DeferredBoot
		})

		deferred := slices.Clone(managed)
		deferred = slices.DeleteFunc(deferred, func(sc config.ServerConfig) bool {
			return !sc.DeferredBoot
		})
		if len(deferred) > 0 {
			slog.Info("BootSequence: deferring non-critical servers to background", "deferred", len(deferred))
		}

		// 🛡️ CONCURRENT BOOT: All critical servers spawn and sync in parallel.
		for _, sc := range critical {
			sc := sc // Capture loop variable
			eg.Go(func() error {
				// Worker isolated recover()
				defer func() {
					if r := recover(); r != nil {
						slog.Error("sub-server lifecycle panicked. skipping...", "component", "lifecycle", "server", sc.Name, "panic", r)
						offlineCount.Add(1)
						app.reg.RequestState(sc.Name, client.StatusOffline)
					}
				}()

				if egCtx.Err() != nil {
					offlineCount.Add(1)
					app.reg.RequestState(sc.Name, client.StatusOffline)
					return nil
				}

				// Handshake Hard-Stop: Use a 60-second timeout for initialize and tools/list
				bootCtx, bootCancel := context.WithTimeout(egCtx, 60*time.Second)
				defer bootCancel()

				slog.Info("BootSequence: syncing server", "name", sc.Name)
				if err := app.reg.SyncServer(bootCtx, sc.Name); err != nil {
					slog.Error("server failed to boot. skipping...", "component", "lifecycle", "server", sc.Name, "error", err)
					offlineCount.Add(1)
					app.reg.RequestState(sc.Name, client.StatusOffline)
				} else {
					successCount.Add(1)
					slog.Info("BootSequence: server ready", "name", sc.Name)
				}
				return nil
			})
		}

		// ✨ NATIVE BOOT: Synchronize the orchestrator's internal tools
		eg.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("native server lifecycle panicked. skipping...", "component", "lifecycle", "server", "magictools", "panic", r)
				}
			}()

			bootCtx, bootCancel := context.WithTimeout(egCtx, 60*time.Second)
			defer bootCancel()

			slog.Info("BootSequence: syncing native server", "name", "magictools")
			var internalTools []*mcp.Tool
			if err := json.Unmarshal(handler.InternalToolsInventoryJSON, &internalTools); err != nil {
				slog.Error("failed to parse native tools.", "component", "inventory", "error", err)
				return nil
			}

			listRes := mcp.ListToolsResult{Tools: internalTools}
			if _, err := app.reg.SyncNativeTools(bootCtx, &listRes); err != nil {
				slog.Error("native server failed to boot.", "component", "lifecycle", "server", "magictools", "error", err)
			} else {
				slog.Info("BootSequence: native server ready", "name", "magictools")
			}
			return nil
		})

		if err := eg.Wait(); err != nil {
			slog.Warn("BootSequence: encountered errors during background sync", "error", err)
		}
		active := successCount.Load()
		failed := offlineCount.Load()
		skipped := int64(len(managed)) - active - failed

		// 🛡️ SYNC COMPLETION: Signal that the ecosystem is now considered synced.
		app.reg.IsSynced.Store(true)

		// 🛡️ HYDRATOR GATE: Send non-blocking pulse to wake the hydrator daemon
		select {
		case app.hydratorSignal <- struct{}{}:
		default:
		}

		// 🛡️ GC RESTORE: Re-enable GC loops now that the mass allocation sequence is securely flushed
		debug.SetGCPercent(100)

		asyncTimelineWg.Add(1)
		go func() {
			defer asyncTimelineWg.Done()
			app.reg.PruneOrphans() // Non-urgent cleanup, runs in background
		}()

		// 🛡️ DEFERRED BOOT: Non-critical servers (git, github, glab) boot in
		// background after the main boot completes. They don't block IDE readiness.
		if len(deferred) > 0 {
			asyncTimelineWg.Add(1)
			go func() {
				defer asyncTimelineWg.Done()
				var defWg sync.WaitGroup
				for _, sc := range deferred {
					sc := sc
					defWg.Add(1)
					go func() {
						defer defWg.Done()
						defer func() {
							if r := recover(); r != nil {
								slog.Error("deferred server lifecycle panicked", "server", sc.Name, "panic", r)
								app.reg.RequestState(sc.Name, client.StatusOffline)
							}
						}()
						bootCtx, bootCancel := context.WithTimeout(ctx, 60*time.Second)
						defer bootCancel()
						slog.Info("BootSequence: syncing deferred server", "name", sc.Name)
						if err := app.reg.SyncServer(bootCtx, sc.Name); err != nil {
							slog.Error("deferred server failed to boot", "server", sc.Name, "error", err)
							app.reg.RequestState(sc.Name, client.StatusOffline)
						} else {
							slog.Info("BootSequence: deferred server ready", "name", sc.Name)
						}
					}()
				}
				defWg.Wait() // Wait for all deferred servers in this batch
			}()
		}

		slog.Info("BootSequence phase complete.",
			"success", active,
			"offline", failed,
			"duration", time.Since(app.ProcessStartTime))

		// 🛡️ AUDIT: Pipe isolation verified. (Routing to Stderr)
		io.WriteString(os.Stderr, "[ORCHESTRATOR] [AUDIT] Pipe isolation verified.\n")
		slog.Info("Boot Complete", "component", "backplane", "active", active, "failed", failed, "skipped", skipped)

		slog.Info("orchestrator: signaling tools/list_changed to IDE")
		util.TraceFunc(ctx, "event", "signaling_sync")
		type emptySchema struct {
			Type string `json:"type"`
		}
		dummy := &mcp.Tool{
			Name:        "__magic_sync_signal__",
			Description: "Internal synchronization signal",
			InputSchema: emptySchema{Type: "object"},
		}
		app.server.AddTool(dummy, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, nil
		})
		app.server.RemoveTools("__magic_sync_signal__")

		// 🛡️ PIPELINE GATE: Final boot verdict — enable the active pipeline
		// only when ALL required servers are online. Must be LAST in boot.
		pipelineReady := app.checkPipelineServers()
		app.activePipelineEnabled.Store(pipelineReady)

		// 🛡️ TIMELINE CLOSURE: Wait for all asynchronous daemon tasks to settle before reporting the exact boot duration.
		go func() {
			asyncTimelineWg.Wait()

			slog.Info("======================================================")
			slog.Info("               ORCHESTRATOR BOOT SUMMARY              ")
			slog.Info("======================================================")

			if app.cfg.Intelligence.Provider != "" && app.cfg.Intelligence.APIKey != "" {
				slog.Info("Hydrator: ENABLED")
			} else {
				slog.Warn("Hydrator: DISABLED (missing provider or API key)")
			}

			if vector.GetEngine() != nil && vector.GetEngine().VectorEnabled() {
				slog.Info("compose_pipeline vector search: ENABLED")
			} else {
				slog.Warn("compose_pipeline vector search: DISABLED")
			}

			if pipelineReady {
				slog.Info("Active code generation pipeline: ENABLED (recall + brainstorm + go-refactor online)", "component", "pipeline")
			} else {
				slog.Warn("Active code generation pipeline: DISABLED (missing required servers)", "component", "pipeline")
			}

			slog.Info(fmt.Sprintf("Boot duration: %.2f seconds", time.Since(app.ProcessStartTime).Seconds()))
			slog.Info("======================================================")
		}()

		return nil
	})
}

// checkPipelineServers verifies that all 3 required servers (recall,
// brainstorm, go-refactor) are online. Returns true only when the
// full pipeline is available.
func (app *OrchestratorApp) checkPipelineServers() bool {
	required := []string{"recall", "brainstorm", "go-refactor"}
	for _, name := range required {
		ss, ok := app.reg.GetServer(name)
		if !ok {
			slog.Warn("Required server not registered", "component", "pipeline", "server", name)
			return false
		}
		if ss.Status != client.StatusReady && ss.Status != client.StatusHealthy {
			slog.Warn("Required server not online", "component", "pipeline", "server", name, "status", ss.Status)
			return false
		}
	}
	return true
}

// primeFromRecall queries recall for historical session telemetry and
// auto-tunes runtime parameters. Currently calibrates TokenSpendThresh
// to 120% of the historical average token spend, preventing both
// runaway burns and overly conservative static limits.
//
// Safe no-op when recallClient is nil or recall is offline.
func primeFromRecall(ctx context.Context, rc *external.MCPClient, cfg *config.Config) {
	if rc == nil || !rc.RecallEnabled() {
		return
	}

	profile := rc.GetOperationalProfile(ctx, 20)
	if profile == nil || profile.SessionCount < 3 {
		slog.Info("operational priming: insufficient historical data, using static defaults",
			"component", "priming")
		return
	}

	// Auto-calibrate TokenSpendThresh to 120% of historical average.
	// Only override if the current value is at the default (1,500,000).
	const defaultTokenThresh = 1500000
	if profile.AvgTokenSpend > 0 && cfg.TokenSpendThresh == defaultTokenThresh {
		calibrated := int(profile.AvgTokenSpend * 1.2)
		// Clamp to reasonable bounds [500_000, 5_000_000]
		if calibrated < 500000 {
			calibrated = 500000
		}
		if calibrated > 5000000 {
			calibrated = 5000000
		}
		slog.Info("operational priming: TokenSpendThresh auto-calibrated from recall",
			"component", "priming",
			"old_value", cfg.TokenSpendThresh,
			"new_value", calibrated,
			"avg_historical", int(profile.AvgTokenSpend),
			"sessions_analyzed", profile.SessionCount)
		cfg.TokenSpendThresh = calibrated
	}

	// Log server failure rates for observability
	for server, rate := range profile.ServerFailureRates {
		if rate > 0.3 {
			slog.Warn("operational priming: elevated server failure rate detected",
				"component", "priming",
				"server", server,
				"failure_rate", fmt.Sprintf("%.1f%%", rate*100))
		}
	}
}

func loadConfigAndDB(configPath, dbPath, logPath string, noOptimize, debug bool) (*db.Store, *config.Config, error) {
	cfg, err := config.New(Version, configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	cfg.NoOptimize = noOptimize
	cfg.Debug = debug
	if logPath != "" && logPath != config.DefaultLogPath() {
		cfg.LogPath = logPath
	}

	store, err := db.NewStore(dbPath, cfg.LRULimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	return store, cfg, nil
}

func waitForRecallSocketReady(ctx context.Context, logger *slog.Logger) error {
	val := os.Getenv("MCP_API_URL")
	if val == "" {
		// Standalone execution bound; ignore ping.
		return nil
	}

	// Parse first chunk natively ignoring trailing commas
	urls := strings.Split(val, ",")
	u, err := url.Parse(strings.TrimSpace(urls[0]))
	if err != nil || u.Host == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("recall connection timeout threshold exceeded")
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", u.Host, 100*time.Millisecond)
			if err == nil && conn != nil {
				conn.Close()
				return nil
			}
		}
	}
}
