package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"mcp-server-magictools/internal/config"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (h *OrchestratorHandler) registerSyncTools(s *mcp.Server) {
	// Descriptions sourced from inventory.go via addTool().
	h.addTool(s, &mcp.Tool{Name: "sync_ecosystem"}, h.SyncEcosystem)
	h.addTool(s, &mcp.Tool{Name: "sync_server"}, h.SyncServer)
}

// SyncEcosystem is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) SyncEcosystem(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct{}
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		_ = json.Unmarshal(req.Params.Arguments, &args)
	}

	result, err := h.Registry.SyncEcosystem(ctx)
	if err != nil {
		return nil, err
	}

	// 🛡️ NATIVE TOOL INJECTION: Treat magictools as a synthetic sub-server
	var listRes mcp.ListToolsResult
	if parseErr := json.Unmarshal(InternalToolsInventoryJSON, &listRes.Tools); parseErr == nil {
		if _, syncErr := h.Registry.SyncNativeTools(ctx, &listRes); syncErr != nil {
			slog.Error("SyncEcosystem: failed to sync native tools", "error", syncErr)
			result.Failed = append(result.Failed, "magictools")
		} else {
			result.Connected = append(result.Connected, "magictools")
		}
	} else {
		slog.Error("SyncEcosystem: failed to parse native tools directory", "error", parseErr)
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("Ecosystem synchronized. Connected: %d/%d servers.\n",
		len(result.Connected), len(result.Connected)+len(result.Failed)))

	var active []string
	for _, sc := range h.Config.GetManagedServers() {
		active = append(active, sc.Name)
	}
	if count, pruneErr := h.Store.PruneOrphans(active); pruneErr == nil && count > 0 {
		msg.WriteString(fmt.Sprintf("  Metric alignment: Pruned %d orphaned internal sub-server tools structurally.\n", count))
	}

	if len(result.Connected) > 0 {
		msg.WriteString(fmt.Sprintf("  Online: %s\n", strings.Join(result.Connected, ", ")))
	}
	if len(result.Failed) > 0 {
		msg.WriteString(fmt.Sprintf("  Failed: %s\n", strings.Join(result.Failed, ", ")))
	}

	// 🛡️ HYDRATOR HOOK: Send non-blocking pulse to wake the hydrator daemon
	select {
	case h.HydratorSignal <- struct{}{}:
		slog.Debug("SyncEcosystem: signaled hydrator daemon to wake")
	default:
		slog.Debug("SyncEcosystem: hydrator daemon already evaluating limits, signal deduplicated")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: msg.String(),
			},
		},
	}, nil
}

// SyncServer is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) SyncServer(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Name string `json:"name"`
	}
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
		}
	}

	if args.Name == "magictools" {
		var listRes mcp.ListToolsResult
		if parseErr := json.Unmarshal(InternalToolsInventoryJSON, &listRes.Tools); parseErr != nil {
			return nil, fmt.Errorf("failed to parse internal native tools: %w", parseErr)
		}
		if _, syncErr := h.Registry.SyncNativeTools(ctx, &listRes); syncErr != nil {
			return nil, syncErr
		}
	} else {
		if err := h.Registry.SyncServer(ctx, args.Name); err != nil {
			return nil, err
		}
	}

	// 🛡️ HYDRATOR HOOK: Send non-blocking pulse to wake the hydrator daemon
	select {
	case h.HydratorSignal <- struct{}{}:
		slog.Debug("SyncServer: signaled hydrator daemon to wake")
	default:
		slog.Debug("SyncServer: hydrator daemon already evaluating limits, signal deduplicated")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("Server %s synchronized successfully.", args.Name),
			},
		},
	}, nil
}

func (h *OrchestratorHandler) registerMaintenanceTools(s *mcp.Server) {
	// Descriptions sourced from inventory.go via addTool().
	h.addTool(s, &mcp.Tool{Name: "sleep_servers"}, h.SleepServers)
	h.addTool(s, &mcp.Tool{Name: "wake_servers"}, h.WakeServers)
	h.addTool(s, &mcp.Tool{Name: "reload_servers"}, h.ReloadServers)
}

// SleepServers is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) SleepServers(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Proactively explicitly trigger BadgerDB ValueLog GC and Bleve defrag prior to ecosystem sleep events.
	go func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		ps := NewProxyService(h)
		_, _ = ps.ExecuteProxy(timeoutCtx, "recall", "context_vacuum", nil, 120*time.Second)
	}()

	h.Registry.DisconnectAll()
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: "All sub-server processes have been put to sleep. They will reactivate automatically on the next tool call.",
			},
		},
	}, nil
}

// WakeServers is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) WakeServers(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	servers := h.Config.GetManagedServers()

	var (
		alreadyOnline []string
		woken         []string
		failed        []string
		mu            sync.Mutex
		wg            sync.WaitGroup
	)

	for _, sc := range servers {
		if sc.Disabled {
			continue
		}
		if _, ok := h.Registry.GetServerSession(sc.Name); ok {
			alreadyOnline = append(alreadyOnline, sc.Name)
			continue
		}

		wg.Add(1)
		go func(srv config.ServerConfig) {
			defer wg.Done()
			if err := h.Registry.Connect(ctx, srv.Name, srv.Command, srv.Args, srv.Env, srv.Hash()); err != nil {
				slog.Error("wake_servers: JIT activation failed", "server", srv.Name, "error", err)
				mu.Lock()
				failed = append(failed, srv.Name)
				mu.Unlock()
			} else {
				mu.Lock()
				woken = append(woken, srv.Name)
				mu.Unlock()
			}
		}(sc)
	}

	wg.Wait()

	// Verify all servers are responsive
	h.Registry.PingAll(ctx)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("Wake complete. %d/%d servers online.\n", len(alreadyOnline)+len(woken), len(servers)))
	if len(alreadyOnline) > 0 {
		msg.WriteString(fmt.Sprintf("  Already online: %s\n", strings.Join(alreadyOnline, ", ")))
	}
	if len(woken) > 0 {
		msg.WriteString(fmt.Sprintf("  Woken: %s\n", strings.Join(woken, ", ")))
	}
	if len(failed) > 0 {
		msg.WriteString(fmt.Sprintf("  Failed: %s\n", strings.Join(failed, ", ")))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg.String()}},
	}, nil
}

// ReloadServers is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) ReloadServers(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Names string `json:"names"`
	}
	if req.Params != nil && len(req.Params.Arguments) > 0 {
		_ = json.Unmarshal(req.Params.Arguments, &args)
	}

	// Case 1: Full Ecosystem Reload
	if args.Names == "" {
		slog.Info("reload_servers: initiating full ecosystem restart")
		h.Registry.DisconnectAll()
		result, err := h.Registry.SyncEcosystem(ctx)
		if err != nil {
			return nil, err
		}

		var msg strings.Builder
		msg.WriteString(fmt.Sprintf("Ecosystem reloaded and synchronized. Connected: %d/%d servers.\n",
			len(result.Connected), len(result.Connected)+len(result.Failed)))

		if len(result.Connected) > 0 {
			msg.WriteString(fmt.Sprintf("  Online: %s\n", strings.Join(result.Connected, ", ")))
		}
		if len(result.Failed) > 0 {
			msg.WriteString(fmt.Sprintf("  Failed: %s\n", strings.Join(result.Failed, ", ")))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: msg.String()}},
		}, nil
	}

	// Case 2: Selective Parallel Reload
	names := strings.Fields(args.Names)
	slog.Info("reload_servers: initiating selective restart", "targets", names)
	return h.executeSelectiveReload(ctx, names)
}

func (h *OrchestratorHandler) executeSelectiveReload(ctx context.Context, names []string) (*mcp.CallToolResult, error) {
	var (
		restarted []string
		failed    []string
		mu        sync.Mutex
		wg        sync.WaitGroup
	)

	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			h.Registry.DisconnectServer(n, false)
			if err := h.Registry.SyncServer(ctx, n); err != nil {
				mu.Lock()
				failed = append(failed, n)
				mu.Unlock()
			} else {
				mu.Lock()
				restarted = append(restarted, n)
				mu.Unlock()
			}
		}(name)
	}

	wg.Wait()

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("Selective reload complete. %d/%d servers processed.\n", len(restarted), len(names)))
	if len(restarted) > 0 {
		msg.WriteString(fmt.Sprintf("  Online: %s\n", strings.Join(restarted, ", ")))
	}
	if len(failed) > 0 {
		msg.WriteString(fmt.Sprintf("  Offline/Failed: %s\n", strings.Join(failed, ", ")))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg.String()}},
	}, nil
}

// OnServerPromoted handles a server transitioning from magictools-managed to IDE-managed
func (h *OrchestratorHandler) OnServerPromoted(name string) {
	h.Registry.DisconnectServer(name, true)
	if err := h.Store.PurgeServerTools(name); err != nil {
		slog.Warn("Failed to purge server tools on promotion", "server", name, "error", err)
	}
}

// OnServerDemoted handles a server transitioning from IDE-managed to magictools-managed
func (h *OrchestratorHandler) OnServerDemoted(name string) {
	slog.Info("server available for magictools management", "server", name)
}

// OnServerUpdated seamlessly reloads the sub-server process to apply new config parameters
func (h *OrchestratorHandler) OnServerUpdated(name string) {
	slog.Info("hot-reloading server due to parameter changes", "server", name)
	_, _ = h.executeSelectiveReload(context.Background(), []string{name})
}
