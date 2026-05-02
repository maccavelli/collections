package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
	"mcp-server-recall/internal/search"
	"mcp-server-recall/internal/server"
	"mcp-server-recall/internal/util"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Boots the JSON-RPC daemon (Proxy Sub-server)",
	RunE: func(cmd *cobra.Command, args []string) error {
		logBuffer := &server.LogBuffer{}
		cleanupLogs := util.SetupStandardLogging("recall", logBuffer)
		defer cleanupLogs()

		slog.Info("[BACKPLANE] SPAWN "+config.Name,
			"version", Version,
			"execution_mode", "daemon",
			"pid", os.Getpid(),
		)

		// Rely on cleanly initialized OS interruption mappings natively decoupling from SIGHUP.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		reader := bufio.NewReaderSize(os.Stdin, 128*1024)
		writer := bufio.NewWriterSize(RealStdout, 128*1024)

		if err := runServe(ctx, stop, logBuffer, reader, writer); err != nil {
			if isExpectedShutdownErr(err) {
				slog.Info("server shut down gracefully", "error", err)
				if fErr := writer.Flush(); fErr != nil {
					slog.Debug("failed to flush writer during shutdown", "error", fErr)
				}
				return nil
			}
			return fmt.Errorf("server fatal error: %w", err)
		}

		if fErr := writer.Flush(); fErr != nil {
			slog.Debug("best-effort final flush before exit failed", "error", fErr)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}

func isExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{"eof", "broken pipe", "connection reset", "use of closed", "file already closed", "bad file descriptor", "client is closing", "connection closed"} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

func runServe(ctx context.Context, cancel context.CancelFunc, logs *server.LogBuffer, reader io.Reader, writer io.Writer) error {
	store, err := memory.NewMemoryStore(ctx, Cfg.GetDBPath(), Cfg.EncryptionKey(), Cfg.SearchLimit(), Cfg.BatchSettings())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	go func(ctx context.Context) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m := store.GetMetrics()
				slog.Info("cache metrics",
					"hits", m.Hits,
					"misses", m.Misses,
					"entries", m.Entries,
					"memories", m.Memories,
					"sessions", m.Sessions,
					"standards", m.Standards,
					"projects", m.Projects,
					"bleve_docs", m.BleveDocs,
				)
			}
		}
	}(ctx)

	var engine *search.BleveEngine
	if Cfg.SearchEnabled() {
		indexPath := filepath.Join(Cfg.GetDBPath(), "search_index")
		var engineErr error
		engine, engineErr = search.InitStorage(indexPath)
		if engineErr != nil {
			slog.Error("Failed to create or open search engine (continuing without search)", "error", engineErr)
		} else {
			if setErr := store.SetSearchEngine(ctx, engine); setErr != nil {
				slog.Error("Failed to initialize search index (continuing without search)", "error", setErr)
			}
		}
	}

	mcpServer, err := server.NewMCPRecallServer(Cfg, store, logs, slog.Default())
	if err != nil {
		if engine != nil {
			engine.Close()
		}
		return fmt.Errorf("could not launch MCP server: %w", err)
	}

	defer func() {
		if engine != nil {
			slog.Info("Shutting down bleve search index gracefully")
			engine.Close()
		}
	}()

	errChan := make(chan error, 1)

	apiPort := Cfg.APIPort()
	if apiPort > 0 {
		httpServer := startStreamableHTTPAPI(ctx, mcpServer, errChan, apiPort)
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("Streamable HTTP server shutdown error", "error", err)
			}
		}()
	}

	startStdioServer(ctx, cancel, mcpServer, reader, writer, errChan)

	select {
	case <-ctx.Done():
		slog.Info("context cancelled; initiating graceful shutdown")
	case err := <-errChan:
		if isExpectedShutdownErr(err) {
			slog.Info("stdio transport closed gracefully", "reason", err.Error())
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func startStreamableHTTPAPI(ctx context.Context, mcpServer *server.MCPRecallServer, errChan chan<- error, port int) *http.Server {
	slog.Info("starting Streamable HTTP API (MCP 2025-03-26)", "port", port)
	streamHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		readOnly := mcp.NewServer(&mcp.Implementation{
			Name:    config.Name + "-readonly",
			Version: Version,
		}, &mcp.ServerOptions{Logger: slog.Default()})
		mcpServer.RegisterSafeTools(readOnly)
		return readOnly
	}, nil)

	internalStreamHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		internalSrv := mcp.NewServer(&mcp.Implementation{
			Name:    config.Name + "-internal",
			Version: Version,
		}, &mcp.ServerOptions{Logger: slog.Default()})
		mcpServer.RegisterSafeToolsInternal(internalSrv)
		return internalSrv
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", newAuditMiddleware(streamHandler))
	mux.Handle("/mcp/internal", &localhostMiddleware{next: newAuditMiddleware(internalStreamHandler)})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func(ctx context.Context) {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Streamable HTTP server exited", "error", err)
			errChan <- err
		}
	}(ctx)

	return srv
}

type auditMiddleware struct {
	next     http.Handler
	mu       sync.RWMutex
	sessions map[string]string
}

func newAuditMiddleware(next http.Handler) *auditMiddleware {
	return &auditMiddleware{
		next:     next,
		sessions: make(map[string]string),
	}
}

type localhostMiddleware struct {
	next http.Handler
}

func (lm *localhostMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		slog.Warn("Blocked external request to internal CLI endpoint", "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden: internal CLI endpoint is bound to localhost", http.StatusForbidden)
		return
	}
	lm.next.ServeHTTP(w, r)
}

type initializeParams struct {
	ClientInfo struct {
		Name string `json:"name"`
	} `json:"clientInfo"`
}

type jsonRPCEnvelope struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (am *auditMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Streamable HTTP uses the Mcp-Session-Id header for session tracking.
	sessionID := r.Header.Get("Mcp-Session-Id")
	if r.Method == http.MethodPost && r.Body != nil {
		// Use 4MB limit for audit peeking to avoid truncating large session payloads.
		// The previous 64KB limit was silently truncating save_sessions state_data,
		// creating malformed JSON that caused the go-sdk to return HTTP 400 "Bad Request",
		// which permanently killed the client's Streamable HTTP connection.
		body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
		if err == nil && len(body) > 0 {
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
			var envelope jsonRPCEnvelope
			if json.Unmarshal(body, &envelope) == nil {
				switch envelope.Method {
				case "initialize":
					var params initializeParams
					if json.Unmarshal(envelope.Params, &params) == nil && params.ClientInfo.Name != "" {
						if sessionID == "" {
							sessionID = "pre-init"
						}
						am.mu.Lock()
						am.sessions[sessionID] = params.ClientInfo.Name
						am.mu.Unlock()
						slog.Info("Streamable HTTP client identified",
							"session", sessionID,
							"client", params.ClientInfo.Name,
						)
					}
				default:
					if sessionID != "" {
						am.mu.RLock()
						clientName := am.sessions[sessionID]
						am.mu.RUnlock()
						if clientName != "" {
							ctx := util.WithClient(r.Context(), clientName)
							r = r.WithContext(ctx)
						}
					}
				}
			}
		}
	}
	am.next.ServeHTTP(w, r)
}

func startStdioServer(ctx context.Context, cancel context.CancelFunc, mcpServer *server.MCPRecallServer, reader io.Reader, writer io.Writer, errChan chan<- error) {
	go func(ctx context.Context) {
		eofReader := &eofDetector{
			r:      reader,
			cancel: cancel,
		}
		autoWriter := &autoFlusher{w: writer}
		if err := mcpServer.Serve(ctx, util.NopWriteCloser{Writer: autoWriter}, util.NopReadCloser{Reader: eofReader}); err != nil {
			errChan <- err
		}
	}(ctx)
}

type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if err == io.EOF {
		slog.Warn("orchestrator pipe closed (EOF); self-terminating")
		e.cancel()
	}
	return n, err
}

type flusher interface {
	Flush() error
}

type autoFlusher struct {
	w io.Writer
}

func (a *autoFlusher) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if f, ok := a.w.(flusher); ok {
		if fErr := f.Flush(); fErr != nil {
			slog.Debug("auto-flush routine encountered an error", "error", fErr)
		}
	}
	return n, err
}
