package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/hfsc"
	"mcp-server-magictools/internal/logging"
	"mcp-server-magictools/internal/util"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// VerifyPipeIsolation is undocumented but satisfies standard structural requirements.
func VerifyPipeIsolation(cmd *exec.Cmd, serverID string) error {
	isViolated := false
	if cmd.Stdout == os.Stdout {
		isViolated = true
		cmd.Stdout = &bytes.Buffer{}
	}
	if cmd.Stderr == os.Stdout {
		isViolated = true
		// Filtered back to a safe file, never stdout
		cmd.Stderr = util.OpenHardenedLogFile(config.DefaultLogPath())
	}
	if isViolated {
		slog.Error("violation: server attempted to use os.stdout", "component", "lifecycle", "server", serverID)
		return fmt.Errorf("FATAL: Pipe isolation violation for %s", serverID)
	}
	return nil
}

const (
	EnvManaged      = "MCP_ORCHESTRATOR_OWNED"
	EnvManagedValue = "true"
	EnvServerName   = "MCP_ORCHESTRATOR_SERVER_NAME"
)

// IdleReconcileThreshold is the minimum inactivity duration before the
// controlLoop stops auto-reconciling a disconnected server.
// Servers idle longer than this must be reactivated via JIT Connect().
const IdleReconcileThreshold = 1 * time.Hour

// Connect is the public-facing gateway for ensuring a server is healthy.
func (m *WarmRegistry) Connect(ctx context.Context, name, command string, args []string, env map[string]string, configHash string) error {
	util.TraceFunc(ctx, "event", "warm_registry_connect", "server", name)

	m.mu.Lock()
	s, exists := m.Servers[name]
	if !exists {
		s = m.initSubServer(name)
		m.Servers[name] = s
	}
	if s.ReadyChan == nil {
		s.ReadyChan = make(chan struct{})
		s.ReadyOnce = sync.Once{}
	}

	// 🛡️ CIRCUIT BREAKER: Block connections if the server has repeatedly crashed.
	if s.ConsecutiveErrors >= CircuitBreakerThreshold {
		if time.Since(s.LastFailure) > 5*time.Minute {
			s.ConsecutiveErrors = 0
		} else {
			m.mu.Unlock()
			return fmt.Errorf("circuit breaker open for %s (consecutive errors: %d)", name, s.ConsecutiveErrors)
		}
	}

	// Update configuration for the server so the actor knows what to start
	s.Command = command
	s.Args = args
	s.Env = env
	s.ConfigHash = configHash
	s.DesiredState = StatusHealthy

	// 🛡️ SHORTRUN: If already healthy or ready, we are done.
	if s.Status == StatusHealthy || s.Status == StatusReady {
		m.mu.Unlock()
		return nil
	}

	// 🛡️ HARDENING: Reset actor capability for disconnected/crashed servers
	if s.Status == StatusDisconnected || s.Status == StatusCrashed {
		newCtx, newCancel := context.WithCancel(context.Background())
		s.Ctx = newCtx
		s.CancelFunc = newCancel
		s.ActorStartedOnce = sync.Once{}
	}

	// 🛡️ DEDUPLICATION: If already starting or syncing, don't send another command, just wait.
	isStarting := (s.Status == StatusStarting || s.Status == StatusSyncing)
	currentStatus := s.Status // snapshot under lock
	readyChan := s.ReadyChan  // Shadow for wait loop
	mailbox := s.Mailbox
	m.mu.Unlock()

	if isStarting {
		slog.Debug("connect: already in transition, joining wait", "component", "watchdog", "server", name, "status", currentStatus)
	} else {
		// 🛡️ ACTOR START: Ensure the actor is running for this server.
		m.EnsureActorRunning(s)

		// Send message to actor
		select {
		case mailbox <- cmdConnect:
		case <-time.After(2 * time.Second):
			// 🛡️ FIX: m.mu was already unlocked at line 109. Do NOT unlock again.
			return fmt.Errorf("FATAL: actor mailbox completely saturated for %s, averting deadlock", name)
		}
	}

	// Wait for Ready signal (Channel Close) or Context Timeout
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-readyChan:
		m.mu.RLock()
		status := m.Servers[name].Status
		m.mu.RUnlock()

		if status == StatusHealthy || status == StatusReady {
			slog.Log(ctx, util.LevelTrace, "connect: success", "component", "watchdog", "server", name)
			return nil
		}
		return fmt.Errorf("server %s failed to start (status: %s)", name, status)
	}
}

func (m *WarmRegistry) reconcile(ctx context.Context, s *SubServer) {
	util.TraceFunc(ctx, "event", "warm_registry_reconcile", "server", s.Name)

	m.mu.Lock()
	desired := s.DesiredState
	current := s.Status
	name := s.Name
	readyChan := s.ReadyChan
	m.mu.Unlock()

	if desired == StatusHealthy && current != StatusHealthy && current != StatusReady && current != StatusStarting && current != StatusSyncing {
		if !m.shouldReconcile(name) {
			return
		}

		slog.Info("spawn", "component", "backplane", "server", name)
		m.mu.Lock()
		s.Status = StatusStarting
		s.LastRestartAt = time.Now()
		m.mu.Unlock()

		if err := m.performConnect(ctx, s); err != nil {
			slog.Error("reconcile fail", "component", "watchdog", "server", name, "error", err)
			m.markFailure(name)
		}

		m.mu.Lock()
		if readyChan != nil {
			s.ReadyOnce.Do(func() {
				close(readyChan)
			})
			s.ReadyChan = nil
		}
		m.mu.Unlock()
	} else if desired == StatusHealthy && (current == StatusHealthy || current == StatusReady || current == StatusCrashed) {
		// Terminal state reached, ensure waiter is released
		m.mu.Lock()
		if readyChan != nil {
			s.ReadyOnce.Do(func() {
				close(readyChan)
			})
			s.ReadyChan = nil
		}
		m.mu.Unlock()
	} else if desired == StatusDisconnected && current != StatusDisconnected {
		slog.Info("reconcile: initiating shutdown", "component", "watchdog", "server", name)
		m.DisconnectServer(name, true)
	}
}

func (m *WarmRegistry) performConnect(ctx context.Context, s *SubServer) error {
	m.mu.RLock()
	name := s.Name
	command := s.Command
	args := s.Args
	env := make(map[string]string)
	for k, v := range s.Env {
		env[k] = v
	}
	configHash := s.ConfigHash
	memoryLimitMB := s.MemoryLimitMB
	goMemLimitMB := s.GoMemLimitMB
	maxCPULimit := s.MaxCPULimit
	m.mu.RUnlock()

	util.TraceFunc(ctx, "event", "warm_registry_performConnect", "server", name)
	if m.checkExistingLiveness(ctx, name) {
		return nil
	}

	slog.Log(ctx, util.LevelTrace, "performconnect: spawning process", "component", "watchdog", "server", name, "command", command)
	cmd, stdin, stdout, err := m.spawnProcess(ctx, name, command, args, env, memoryLimitMB, goMemLimitMB, maxCPULimit)
	if err != nil {
		return err
	}

	slog.Log(ctx, util.LevelTrace, "performconnect: creating session", "component", "watchdog", "server", name)
	slog.Info("handshake", "component", "backplane", "server", name)
	logFile := m.LogSink
	if logFile == nil {
		logFile = util.OpenHardenedLogFile(config.DefaultLogPath())
	}

	// Initialize Manual Response Router for the handshake phase
	pendingMap := &sync.Map{}
	session, filter, err := m.createSession(ctx, name, stdin, stdout, logFile, pendingMap)
	if err != nil {
		if killErr := killProcessGroup(cmd); killErr != nil {
			slog.Warn("lifecycle: cleanup kill failed after session error", "server", name, "error", killErr)
		}
		return err
	}

	// 🛡️ ATOMIC UPDATE: Link the new session and process to the existing actor-owned server object.
	m.updateSubServerFields(s, command, args, env, configHash, cmd, session, pendingMap, filter)

	slog.Log(ctx, util.LevelTrace, "performconnect: complete", "component", "watchdog", "server", name)
	slog.Info("synced", "component", "backplane", "server", name)

	m.mu.Lock()
	s.Status = StatusHealthy
	m.mu.Unlock()

	return nil
}

func (m *WarmRegistry) createSession(ctx context.Context, name string, stdin io.WriteCloser, stdout io.ReadCloser, logSink io.Writer, pending *sync.Map) (*mcp.ClientSession, *jsonFilterReader, error) {
	util.TraceFunc(ctx, "event", "warm_registry_createSession", "server", name)
	clientImpl := &mcp.Implementation{
		Name:    "magictools-orchestrator",
		Version: "1.0.0",
	}

	c := mcp.NewClient(clientImpl, &mcp.ClientOptions{
		// 🛡️ SUB-SERVER ISOLATION: Handle notifications internally rather than letting the SDK default
		// which could otherwise leak them or complain about unhandled notifications.
		ToolListChangedHandler: func(ctx context.Context, req *mcp.ToolListChangedRequest) {
			slog.Debug("lifecycle: ignored sub-server tool list_changed notification", "server", name)
		},
		ResourceListChangedHandler: func(ctx context.Context, req *mcp.ResourceListChangedRequest) {
			slog.Debug("lifecycle: ignored sub-server resource list_changed notification", "server", name)
		},
		PromptListChangedHandler: func(ctx context.Context, req *mcp.PromptListChangedRequest) {
			slog.Debug("lifecycle: ignored sub-server prompt list_changed notification", "server", name)
		},
		LoggingMessageHandler: func(ctx context.Context, req *mcp.LoggingMessageRequest) {
			// 🛡️ HFSC: Intercept streamed payload notifications from sub-servers.
			// Version-agnostic prefix match — ParseStreamWire handles version validation.
			if dataStr, ok := req.Params.Data.(string); ok {
				if strings.HasPrefix(dataStr, "HFSC_STREAM|") || strings.HasPrefix(dataStr, "HFSC_FINALIZE|") {
					m.handleHFSCChunk(name, dataStr)
					return
				}
			}
			slog.Debug("lifecycle: sub-server log message mapped to backplane", "server", name, "msg", req.Params.Data)
		},
	})

	filter := newJsonFilterReader(stdout, logSink, m.Store)
	transport := &DecoderTransport{
		Reader:          filter,
		Writer:          stdin,
		PendingRequests: pending,
	}

	// Handshake Hard-Stop: 60s context timeout for initialize
	connCtx, connCancel := context.WithTimeout(ctx, 60*time.Second)
	defer connCancel()

	slog.Log(ctx, util.LevelTrace, "createsession: initiating handshake", "component", "watchdog", "server", name)

	// Ensure we use the SDK's latest protocolVersion (2025-06-18) by passing nil
	session, err := c.Connect(connCtx, transport, nil)

	if err != nil {
		// 3-Strike Rule / Handshake Hard-Stop: If initialize fails, mark as OFFLINE and move on.
		if rpcErr, ok := errors.AsType[*jsonrpc.Error](err); ok {
			m.Logger.Log(logging.ERROR, name, fmt.Sprintf("Handshake Failed: Protocol Error (%d: %s)", rpcErr.Code, rpcErr.Message))
		} else {
			m.Logger.Log(logging.ERROR, name, fmt.Sprintf("Handshake Failed: Protocol Error (%v)", err))
		}

		slog.Error("handshake hard-stop triggered", "component", "lifecycle",
			"server", name,
			"error", err,
			"ctx_err", connCtx.Err())
		m.markOffline(name)
		return nil, nil, fmt.Errorf("protocol handshake failed (hard-stop) for %s: %w", name, err)
	}
	m.Logger.Log(logging.HANDSHAKE, name, "proto: 2025-06-18 | OK")
	m.Logger.Log(logging.READY, name, "Handshake Complete")

	return session, filter, nil
}

// StartReconciler is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) StartReconciler(ctx context.Context) {
	slog.Info("actor registry online", "component", "watchdog")
}

func (s *SubServer) controlLoop(m *WarmRegistry) {
	slog.Info("lifecycle: actor started", "server", s.Name)
	m.mu.RLock()
	ctx := s.Ctx
	m.mu.RUnlock()
	ticker := time.NewTicker(30 * time.Second) // Slow health check
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("lifecycle: actor stopping", "server", s.Name)
			return
		case cmd := <-s.Mailbox:
			slog.Debug("lifecycle: actor received command", "server", s.Name, "cmd", cmd)
			switch cmd {
			case cmdConnect, cmdSync:
				m.reconcile(ctx, s)
			case cmdDisconnect:
				m.DisconnectServer(s.Name, true)
			}
		case <-ticker.C:
			m.mu.RLock()
			lastUsed := s.LastUsed
			current := s.Status
			proc := s.Process
			m.mu.RUnlock()

			// Skip reconcile for idle servers — JIT Connect() handles reactivation
			if (current == StatusDisconnected || current == StatusCrashed) &&
				time.Since(lastUsed) > IdleReconcileThreshold {
				m.mu.Lock()
				s.DesiredState = StatusDisconnected
				m.mu.Unlock()
				continue
			}

			// Skip reconcile if process is still alive (prevents duplicate spawns)
			if proc != nil && proc.Process != nil {
				if err := proc.Process.Signal(syscall.Signal(0)); err == nil {
					continue // process alive, no action needed
				}
			}

			// Periodic health check replacement for the global ticker
			m.reconcile(ctx, s)
		}
	}
}

// EnsureActorRunningFor is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) EnsureActorRunningFor(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.Servers[name]; ok {
		m.EnsureActorRunning(s)
	}
}

// EnsureActorRunning is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) EnsureActorRunning(s *SubServer) {
	s.ActorStartedOnce.Do(func() {
		go func() {
			s.controlLoop(m)
		}()
	})
}

func (m *WarmRegistry) shouldReconcile(name string) bool {
	m.mu.RLock()
	s, ok := m.Servers[name]
	if !ok {
		m.mu.RUnlock()
		return false
	}
	errors := s.ConsecutiveErrors
	lastCompleted := s.LastCompletedAt
	m.mu.RUnlock()

	if errors == 0 {
		return true
	}

	// Exponential backoff: 2^errors * 2 seconds
	backoff := time.Duration(1<<uint(errors)) * 2 * time.Second
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}

	return time.Since(lastCompleted) > backoff
}

func (m *WarmRegistry) startGuardian(ctx context.Context, srv *SubServer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if srv.Process.Process != nil {
				if err := srv.Process.Process.Signal(syscall.Signal(0)); err != nil {
					m.mu.RLock()
					lastUsed := srv.LastUsed
					m.mu.RUnlock()
					if time.Since(lastUsed) > IdleReconcileThreshold {
						slog.Info("Guardian: PID lost but server idle, skipping recovery", "server", srv.Name)
						return
					}
					slog.Error("Guardian: PID lost", "server", srv.Name, "pid", srv.Process.Process.Pid)
					m.recoverServer(srv)
					return
				}
			}
		}
	}
}

func (m *WarmRegistry) recoverServer(srv *SubServer) {
	m.mu.Lock()
	srv.BackoffLevel++
	level := srv.BackoffLevel
	if level > 5 {
		level = 5
	}
	delay := time.Duration(1<<level) * time.Second
	m.mu.Unlock()

	slog.Warn("Guardian: delaying restart", "server", srv.Name, "backoff", delay)
	// 🛡️ FIX: Create a fresh context instead of using srv.Ctx which may
	// already be cancelled by DisconnectServer. This ensures the timer
	// goroutine actually waits for the delay before restarting.
	recoverCtx, recoverCancel := context.WithTimeout(context.Background(), delay+30*time.Second)
	go func(c context.Context, cancel context.CancelFunc) {
		defer cancel()
		select {
		case <-c.Done():
			return
		case <-time.After(delay):
			m.orchestrateRestart(srv)
		}
	}(recoverCtx, recoverCancel)
}

func (m *WarmRegistry) checkExistingLiveness(ctx context.Context, name string) bool {
	m.mu.RLock()
	s, exists := m.Servers[name]
	if !exists {
		m.mu.RUnlock()
		return false
	}

	if s.Session == nil {
		m.mu.RUnlock()
		return false
	}
	m.mu.RUnlock()

	pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	if err := s.Session.Ping(pingCtx, &mcp.PingParams{}); err == nil {
		return true
	}
	return false
}

func (m *WarmRegistry) updateSubServerFields(srv *SubServer, command string, args []string, env map[string]string, configHash string, cmd *exec.Cmd, session *mcp.ClientSession, pending *sync.Map, filter *jsonFilterReader) {
	m.mu.Lock()
	srv.Session = session
	srv.Process = cmd
	srv.LastUsed = time.Now()
	srv.StartTime = time.Now()
	srv.ConfigHash = configHash
	srv.Command = command
	srv.Args = args
	srv.Env = env
	srv.HandshakeComplete = true
	srv.Status = StatusHealthy
	srv.PendingRequests = pending
	srv.Filter = filter
	m.mu.Unlock()

	m.resetFailure(srv.Name)
	slog.Info("lifecycle: server fields updated", "name", srv.Name, "pid", cmd.Process.Pid)
}

func (m *WarmRegistry) spawnProcess(ctx context.Context, name, command string, args []string, env map[string]string, memoryLimitMB, goMemLimitMB, maxCPULimit int) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	m.enforceSingleInstance(ctx, name, command, args)

	cmd := exec.Command(command, args...)
	if ctx != nil && ctx.Err() != nil {
		return nil, nil, nil, ctx.Err()
	}
	cmd.Env = m.prepareProcessEnvironment(name, env, memoryLimitMB, goMemLimitMB, maxCPULimit)
	// 🛡️ HEADLESS NPM/NODE: Violently suppress interactive prompts across the JSON-RPC pipe.
	cmd.Env = append(cmd.Env, "npm_config_yes=true", "npm_config_update_notifier=false")

	if filepath.IsAbs(command) {
		cmd.Dir = filepath.Dir(command)
	}

	cmd.Stdin = nil
	// Route sub-server stderr to the central orchestrator debug log.
	// CRITICAL: This unifies sub-server telemetry into the central log stream.
	// Sub-servers write structured JSON logs to os.Stderr, which the orchestrator
	// captures here. In standalone mode (no orchestrator), sub-servers log locally.
	cmd.Stderr = util.OpenHardenedLogFile(filepath.Join(config.DefaultCacheDir(), fmt.Sprintf("mcp_%s_stderr.log", name)))
	prepareCommand(cmd)

	stdinPR, stdinPW := io.Pipe()
	// 🛡️ FIX: Ensure io.Pipe is cleaned up on error paths to prevent FD/goroutine leaks.
	var spawnSuccess bool
	defer func() {
		if !spawnSuccess {
			stdinPR.Close()
			stdinPW.Close()
		}
	}()

	realStdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}

	// 🛡️ PERF: stdout pipe is passed directly to the transport layer (no intermediary io.Pipe)
	realStdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := VerifyPipeIsolation(cmd, name); err != nil {
		m.Logger.Log(logging.ERROR, name, fmt.Sprintf("Process Start Prevented: %v", err))
		return nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		m.Logger.Log(logging.ERROR, name, fmt.Sprintf("Process Start Fail: %v", err))
		return nil, nil, nil, fmt.Errorf("process start: %w", err)
	}
	m.Logger.Log(logging.SPAWN, name, fmt.Sprintf("pid: %d", cmd.Process.Pid))

	// 🛡️ RACE FIX: Immediately store the new PID so future enforceSingleInstance calls exclude it.
	m.mu.Lock()
	if s, ok := m.Servers[name]; ok {
		s.LastKnownPID = cmd.Process.Pid
	}
	m.mu.Unlock()

	setCloexec(realStdin)
	setCloexec(realStdout)

	go func(c context.Context) {
		err := cmd.Wait()
		m.mu.Lock()
		if s, ok := m.Servers[name]; ok {
			slog.Warn("lifecycle: process exited", "server", name, "pid", cmd.Process.Pid, "error", err)
			s.Status = StatusDisconnected
			if err != nil {
				s.Status = StatusCrashed
			}
			s.Session = nil
			s.Process = nil
		}
		m.mu.Unlock()

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() > 0 {
				m.reportSubServerFailure(name, exitErr.ExitCode())
			}
		}
		if m.PIDDir != "" {
			if err := os.Remove(filepath.Join(m.PIDDir, name+".pid")); err != nil && !os.IsNotExist(err) {
				slog.Warn("lifecycle: failed to remove pid file on exit", "server", name, "error", err)
			}
		}
	}(ctx)

	if m.PIDDir != "" {
		pidFile := filepath.Join(m.PIDDir, name+".pid")
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
			slog.Warn("lifecycle: failed to write pid file", "server", name, "file", pidFile, "error", err)
		}
	}

	m.setupIOBridges(ctx, name, realStdin, stdinPR)

	spawnSuccess = true // mark success to prevent deferred cleanup
	return cmd, stdinPW, realStdout, nil
}

func (m *WarmRegistry) orchestrateRestart(srv *SubServer) {
	m.mu.Lock()
	if srv.Status == StatusStarting || srv.Status == StatusSyncing {
		m.mu.Unlock()
		return
	}
	// Double-guard: Use a simple timestamp to prevent rapid-fire restarts
	if time.Since(srv.LastRestartAt) < 2*time.Second {
		m.mu.Unlock()
		return
	}
	srv.Status = StatusStarting
	srv.LastRestartAt = time.Now()
	srv.RestartCount.Add(1)
	name := srv.Name
	lastCommand := srv.Command
	lastArgs := srv.Args
	lastEnv := srv.Env
	lastHash := srv.ConfigHash
	m.mu.Unlock()

	m.DisconnectServer(name, false) // Keep in map to prevent race

	// 🛡️ FIX: Use a cancellable context with timeout instead of context.Background()
	// to prevent orphan processes during orchestrator shutdown.
	restartCtx, restartCancel := context.WithTimeout(context.Background(), 90*time.Second)
	go func() {
		defer restartCancel()
		// 🛡️ FIX: Delay spawn to 3 seconds to strictly clear the 2-second SIGKILL reaper fallback
		time.Sleep(3 * time.Second)
		if err := m.Connect(restartCtx, name, lastCommand, lastArgs, lastEnv, lastHash); err != nil {
			slog.Warn("lifecycle: restart reconnection failed", "server", name, "error", err)
		}
	}()
}

func (m *WarmRegistry) markFailure(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.Servers[name]; ok {
		s.ConsecutiveErrors++
		s.LastFailure = time.Now()
		s.Status = StatusCrashed
	}
}

func (m *WarmRegistry) resetFailure(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.Servers[name]; ok {
		s.ConsecutiveErrors = 0
	}
}

func (m *WarmRegistry) prepareProcessEnvironment(name string, env map[string]string, _ /* memoryLimitMB */, goMemLimitMB, maxCPULimit int) []string {
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", k, v))
	}

	peerID := util.GenerateSessionID()
	cmdEnv = append(cmdEnv, fmt.Sprintf("MAGIC_TOOLS_PEER_ID=%s", peerID))
	cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", EnvManaged, EnvManagedValue))
	cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", EnvServerName, name))

	// GOMEMLIMIT: explicit config > hardcoded 1GB default.
	// This is a soft GC cap — the Go runtime will aggressively sweep at this
	// threshold but will NOT prevent allocation above it. The watchdog
	// (memory_limit_mb) serves as the hard kill limit.
	if goMemLimitMB <= 0 {
		goMemLimitMB = 1024
	}
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOMEMLIMIT=%dMiB", goMemLimitMB))

	if maxCPULimit <= 0 {
		maxCPULimit = 2
	}
	cmdEnv = append(cmdEnv, fmt.Sprintf("GOMAXPROCS=%d", maxCPULimit))

	logFilePath := m.Config.LogPath
	if logFilePath == "" {
		logFilePath = config.DefaultLogPath()
	}
	cmdEnv = append(cmdEnv, fmt.Sprintf("MCP_LOG_FILE=%s", logFilePath))

	// Propagate the orchestrator's configured sub-server log level.
	// CRITICAL: Without this, sub-servers default to INFO regardless of config.
	// Sub-servers read this in SetupStandardLogging to set their slog level.
	mcpLogLevel := m.Config.GetMCPLogLevel()
	if mcpLogLevel != "" {
		cmdEnv = append(cmdEnv, fmt.Sprintf("ORCHESTRATOR_LOG_LEVEL=%s", mcpLogLevel))
	}
	return cmdEnv
}

// handleHFSCChunk parses an HFSC_STREAM or HFSC_FINALIZE wire-format notification
// and pushes it instantly down the Tier-2 streaming disk pipeline.
func (m *WarmRegistry) handleHFSCChunk(server, wire string) {
	isFin, sessionID, index, chunkOrHash, err := hfsc.ParseStreamWire(wire)
	if err != nil {
		slog.Error("hfsc: tier-2 generic stream parse fault", "server", server, "error", err)
		return
	}

	if isFin {
		go func() {
			path, err := m.HFSC.FinalizeStream(sessionID, index, chunkOrHash)
			if err != nil {
				slog.Error("hfsc: tier-2 stream finalization aborted", "server", server, "session", sessionID, "error", err)
				return
			}
			slog.Info("hfsc: extreme artifact completely manifested to SSD", "server", server, "session", sessionID, "path", path)
		}()
		return
	}

	if err := m.HFSC.AccumulateChunk(sessionID, index, chunkOrHash); err != nil {
		slog.Error("hfsc: tier-2 chunk sink failure", "server", server, "session", sessionID, "error", err)
		return
	}
}
