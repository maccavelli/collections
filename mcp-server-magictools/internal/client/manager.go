package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"os"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/hfsc"
	"mcp-server-magictools/internal/logging"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxRunningServers is the LRU eviction threshold.
const MaxRunningServers = 50

// CircuitBreakerThreshold is the number of consecutive errors before the circuit opens.
const CircuitBreakerThreshold = 3

// BootSummary contains the results of the entire boot sequence.
type BootSummary struct {
	TotalAttempted int
	Success        int
	Limping        int
	Failed         int
	StartTime      time.Time
}

// WarmRegistry handles a pool of sub-servers and ecosystem synchronization.
// It is the SOLE OWNER of sub-process lifecycles.
type WarmRegistry struct {
	Servers           map[string]*SubServer
	HFSC              *hfsc.Registry
	PIDDir            string
	Config            *config.Config
	Store             *db.Store
	Logger            *logging.BackplaneLogger
	LogSink           io.Writer // 🛡️ Troubleshooting: Redirects all sub-server communication
	IsSynced          atomic.Bool
	lastConfigModTime time.Time
	mu                sync.RWMutex
}

// NewWarmRegistry creates the single-owner registry.
func NewWarmRegistry(pidDir string, store *db.Store, cfg *config.Config) *WarmRegistry {
	return &WarmRegistry{
		Servers:           make(map[string]*SubServer),
		HFSC:              hfsc.NewRegistry(config.DefaultCacheDir()),
		PIDDir:            pidDir,
		Store:             store,
		Config:            cfg,
		Logger:            logging.Default,
		lastConfigModTime: time.Now(),
	}
}

// GetServer is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) GetServer(name string) (*SubServer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.Servers[name]
	return s, ok
}

// GetServerSession atomically snapshots a server's Session under RLock.
// The returned *mcp.ClientSession is safe to use after the lock is released
// because Go's GC keeps the object alive as long as any reference exists.
// If the session is closed concurrently, CallTool/Ping will return an error.
func (m *WarmRegistry) GetServerSession(name string) (*mcp.ClientSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.Servers[name]
	if !ok || s.Session == nil {
		return nil, false
	}
	return s.Session, true
}

// HasServer is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) HasServer(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.Servers[name]
	return ok
}

// RLock is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) RLock() {
	m.mu.RLock()
}

// RUnlock is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) RUnlock() {
	m.mu.RUnlock()
}

// RequestState is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) RequestState(name string, state Status) {
	m.mu.Lock()
	s, ok := m.Servers[name]
	if !ok {
		s = m.initSubServer(name)
		m.Servers[name] = s
	}
	s.DesiredState = state
	mailbox := s.Mailbox
	m.mu.Unlock()

	select {
	case mailbox <- cmdConnect:
	default:
	}
}

// Boot ensures all managed servers are requested to be healthy.
func (m *WarmRegistry) Boot(ctx context.Context, servers []config.ServerConfig) {
	slog.Info("WarmRegistry: initiating Boot sequence", "total", len(servers))
	for _, srv := range servers {
		m.mu.Lock()
		s, ok := m.Servers[srv.Name]
		if !ok {
			s = m.initSubServer(srv.Name)
			m.Servers[srv.Name] = s
		}
		s.Command = srv.Command
		s.Args = srv.Args
		s.Env = srv.Env
		s.MemoryLimitMB = srv.MemoryLimitMB
		s.GoMemLimitMB = srv.GoMemLimitMB
		s.MaxCPULimit = srv.MaxCPULimit
		s.ConfigHash = srv.Hash()
		s.DesiredState = StatusHealthy
		mailbox := s.Mailbox
		m.mu.Unlock()

		select {
		case mailbox <- cmdConnect:
		default:
		}
	}
}

func (m *WarmRegistry) initSubServer(name string) *SubServer {
	ctx, cancel := context.WithCancel(context.Background())
	s := &SubServer{
		Name:            name,
		Status:          StatusDisconnected,
		ReadyChan:       make(chan struct{}),
		Mailbox:         make(chan subServerCmd, 10),
		Ctx:             ctx,
		CancelFunc:      cancel,
		PendingRequests: &sync.Map{},
	}
	return s
}

// CallProxy executes a tool on a sub-server.
// Session is snapshotted under RLock to prevent a race with lifecycle teardown.
func (m *WarmRegistry) CallProxy(ctx context.Context, serverName, toolName string, arguments map[string]any, timeout time.Duration) (*mcp.CallToolResult, error) {
	sess, ok := m.GetServerSession(serverName)
	if !ok {
		return nil, fmt.Errorf("server %s not running", serverName)
	}

	// 🛡️ ACTIVE-CALL TRACKING: Increment before call, decrement after.
	// The health monitor checks this counter before triggering mid-flight restarts.
	m.mu.RLock()
	srv := m.Servers[serverName]
	m.mu.RUnlock()
	if srv != nil {
		srv.ActiveCalls.Add(1)
		defer srv.ActiveCalls.Add(-1)
	}

	// Decouple from parent MCP transport context to prevent its deadline
	// from overriding the tool-specific timeout. The parent ctx deadline
	// (AI agent → orchestrator) is typically shorter than long-running
	// tool executions (e.g., go test across many packages).
	callCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	telemetry.LifecycleEvents.BackpressurePending.Add(1)
	defer telemetry.LifecycleEvents.BackpressurePending.Add(-1)

	start := time.Now()
	res, err := sess.CallTool(callCtx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	latency := time.Since(start)

	if err == nil {
		m.mu.Lock()
		if s, ok := m.Servers[serverName]; ok {
			s.LastUsed = time.Now()
			s.TotalCalls++
			s.LastLatency = latency
			s.ConsecutiveErrors = 0
		}
		m.mu.Unlock()
		m.EvictLRU(serverName)
	} else {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timeout") {
			telemetry.LifecycleEvents.BackpressureReject.Add(1)
		}

		// 🛡️ EOF DIAGNOSTIC ENRICHMENT: When the stdio pipe closes unexpectedly,
		// capture process state to determine if the sub-server was OOM-killed,
		// health-monitor restarted, or crashed independently.
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
			telemetry.ErrorTaxonomy.PipeError.Add(1)
			diag := m.diagnoseEOF(serverName)
			slog.Error("proxy: EOF detected on sub-server pipe",
				"component", "proxy",
				"server", serverName,
				"tool", toolName,
				"latency_ms", latency.Milliseconds(),
				"diagnostic", diag,
			)
			m.markFailure(serverName)
			return nil, fmt.Errorf("%s EOF (%s): %w", serverName, diag, err)
		}

		m.markFailure(serverName)
		return nil, err
	}

	return res, nil
}

// diagnoseEOF captures forensic data when an EOF is detected on a sub-server's
// stdio pipe. Returns a human-readable diagnostic string including PID, process
// state, exit status, and server lifecycle status.
func (m *WarmRegistry) diagnoseEOF(serverName string) string {
	m.mu.RLock()
	srv, ok := m.Servers[serverName]
	if !ok {
		m.mu.RUnlock()
		return "server not in registry"
	}
	status := srv.Status
	pid := srv.LastKnownPID
	proc := srv.Process
	activeCalls := srv.ActiveCalls.Load()
	m.mu.RUnlock()

	processState := "unknown"
	exitInfo := ""
	if proc != nil && proc.Process != nil {
		if proc.ProcessState != nil {
			// Process has exited — capture exit status
			processState = "DEAD"
			exitInfo = fmt.Sprintf(", exit: %s", proc.ProcessState.String())
		} else {
			// Check if still alive via signal 0
			if err := proc.Process.Signal(syscall.Signal(0)); err != nil {
				processState = "DEAD"
			} else {
				processState = "ALIVE"
			}
		}
	} else {
		processState = "NO_PROCESS"
	}

	return fmt.Sprintf("pid=%d %s, status=%s, active_calls=%d%s", pid, processState, status, activeCalls, exitInfo)
}

// AuditGlobalRegistry verifies that no internally managed component is holding a reference to os.Stdout.
func (m *WarmRegistry) AuditGlobalRegistry() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	safeWriter := m.LogSink
	if safeWriter == nil {
		logPath := m.Config.LogPath
		if logPath == "" {
			logPath = config.DefaultLogPath()
		}
		safeWriter = util.OpenHardenedLogFile(logPath)
	}

	for name, srv := range m.Servers {
		if srv.Process != nil {
			if srv.Process.Stdout == os.Stdout {
				slog.Error("server process holding os.stdout leak", "component", "manager", "server", name)
				srv.Process.Stdout = safeWriter
			}
			if srv.Process.Stderr == os.Stdout {
				slog.Error("server stderr holding os.stdout leak", "component", "manager", "server", name)
				srv.Process.Stderr = safeWriter
			}
		}
	}
}
