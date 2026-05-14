package cmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"mcp-server-socratic-thinker/internal/handler"
	"mcp-server-socratic-thinker/internal/singleton"
	"mcp-server-socratic-thinker/internal/socratic"
	"mcp-server-socratic-thinker/internal/telemetry"
)

var (
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Socratic Thinker MCP server",
	Run: func(cmd *cobra.Command, args []string) {
		// Singleton enforcement — acquire workspace-scoped UDS lock.
		// Must be first: assassinates any zombie instance before we init anything.
		lockLn := singleton.AcquireLock()
		defer lockLn.Close()

		// Defense-in-depth
		if _, exists := os.LookupEnv("GOMEMLIMIT"); !exists {
			_ = os.Setenv("GOMEMLIMIT", "1024MiB")
		}
		if _, exists := os.LookupEnv("GOMAXPROCS"); !exists {
			_ = os.Setenv("GOMAXPROCS", "2")
		}

		realStdout := os.Stdout
		os.Stdout = os.Stderr

		ringBuffer := telemetry.NewRingBuffer(1000)
		multiWriter := io.MultiWriter(os.Stderr, ringBuffer)
		slog.SetDefault(slog.New(slog.NewTextHandler(multiWriter, nil)))
		slog.Info("Starting Socratic Thinker MCP Server")

		rootCtx := context.Background()
		ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer stop()

		machine := socratic.NewMachine()

		mcpServer := mcp.NewServer(&mcp.Implementation{Name: "socratic-thinker", Version: Version}, &mcp.ServerOptions{
			Logger: slog.Default(),
		})

		handler.Register(mcpServer, machine, ringBuffer)

		startTime := time.Now()

		// Background telemetry server (UDP listener — dashboard connects to us)
		telemetryServer := telemetry.NewServer()
		if telemetryServer != nil {
			telemetryServer.Start()
			defer telemetryServer.Close()
		}

		// UDP emission goroutine — builds domain-specific payload and broadcasts
		go func() {
			if telemetryServer == nil {
				return
			}
			ticker := time.NewTicker(telemetry.EmissionInterval)
			defer ticker.Stop()

			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats) // Initial hydration
			tickCount := 0

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					tickCount++
					// ReadMemStats every 4th tick (2s cadence) to reduce STW overhead
					if tickCount%4 == 0 {
						runtime.ReadMemStats(&memStats)
					}

					stage, trifectaCount, contextBytes, tokensEst := machine.GetMetrics()

					payload := telemetry.MetricPayload{
						UptimeSeconds:       int64(time.Since(startTime).Seconds()),
						MemoryAllocBytes:    memStats.Alloc,
						ActiveGoroutines:    runtime.NumGoroutine(),
						GCPauseNs:           memStats.PauseTotalNs,
						NetworkBytesRead:    bytesRead.Load(),
						NetworkBytesWritten: bytesWritten.Load(),
						PipelineStage:       stage,
						TrifectaReviewCount: trifectaCount,
						SessionContextBytes: contextBytes,
						SessionTokensEst:    tokensEst,
					}

					telemetryServer.Broadcast(payload)
				}
			}
		}()

		reader := bufio.NewReaderSize(os.Stdin, 512*1024)
		writer := bufio.NewWriterSize(realStdout, 512*1024)

		errChan := make(chan error, 1)
		go func(threadCtx context.Context) {
			eofReader := &eofDetector{
				r:      reader,
				cancel: stop,
			}
			autoWriter := &autoFlusher{w: writer}

			t := &mcp.IOTransport{
				Reader: eofReader,
				Writer: autoWriter,
			}
			if _, err := mcpServer.Connect(threadCtx, t, nil); err != nil {
				select {
				case errChan <- err:
				case <-threadCtx.Done():
				}
			}
		}(ctx)

		select {
		case <-ctx.Done():
			slog.Info("context cancelled; initiating graceful shutdown")
		case err := <-errChan:
			if isExpectedShutdownErr(err) {
				slog.Info("stdio transport closed gracefully", "reason", err.Error())
			} else {
				slog.Error("server fatal error", "error", err)
				os.Exit(1)
			}
		}

		_ = writer.Flush()
	},
}

func isExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{"eof", "broken pipe", "connection reset", "use of closed", "file already closed", "client is closing", "connection closed"} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

type eofDetector struct {
	r      io.Reader
	cancel context.CancelFunc
}

func (e *eofDetector) Close() error {
	return nil
}

func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	bytesRead.Add(int64(n))
	if errors.Is(err, io.EOF) {
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
	bytesWritten.Add(int64(n))
	if len(p) > 0 && p[len(p)-1] == '\n' {
		if f, ok := a.w.(flusher); ok {
			_ = f.Flush()
		}
	}
	return n, err
}

func (a *autoFlusher) Close() error {
	if f, ok := a.w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}
