package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/handler"
	"mcp-server-magicskills/internal/scanner"
)

// Version is the build version of the application.
var Version = "2.1.0"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mcp-server-magicskills version %s\n", Version)
		os.Exit(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := execute(ctx); err != nil {
		slog.Error("server fatal error", "error", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context) error {
	// Initialize logging
	logBuffer := &handler.LogBuffer{}
	mw := io.MultiWriter(os.Stderr, logBuffer)
	logger := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Determine Roots
	homePath, _ := os.UserHomeDir()
	globalPath := os.Getenv("MAGIC_SKILLS_PATH")
	if globalPath == "" {
		globalPath = filepath.Join(homePath, "gitrepos/saxsmith-global-context/.agent/skills")
	}

	eng := engine.NewEngine()
	h := &handler.MagicSkillsHandler{
		Engine: eng,
		Logs:   logBuffer,
	}

	roots := []string{globalPath}
	if local, ok := scanner.FindProjectSkillsRoot(); ok {
		roots = append([]string{local}, roots...)
		slog.Info("found local workspace skills", "path", local)
	}

	s, err := scanner.NewScanner(roots)
	if err != nil {
		return fmt.Errorf("scanner init: %w", err)
	}
	defer s.Watcher.Close() // Cleanup for production

	// Initial Ingestion
	files, _ := s.Discover()
	if err := eng.Ingest(files); err != nil {
		slog.Error("initial ingestion failed", "error", err)
	}
	slog.Info("engine ready", "skillsCount", len(eng.Skills), "version", Version)

	// Setup MCP Server
	mcpSrv := server.NewMCPServer("mcp-server-magicskills", Version, server.WithLogging())

	// Registration
	mcpSrv.AddTool(mcp.NewTool("magicskills_list", mcp.WithDescription("Lists all available skills.")), h.HandleListSkills)
	mcpSrv.AddTool(mcp.NewTool("magicskills_get", mcp.WithDescription("Retrieves a full skill's instructions."), 
		mcp.WithString("name", mcp.Description("The name of the skill to retrieve"), mcp.Required())), h.HandleGetSkill)
	mcpSrv.AddTool(mcp.NewTool("magicskills_summarize", mcp.WithDescription("Retrieves a pruned version of a skill."), 
		mcp.WithString("name", mcp.Description("The name of the skill to summarize"), mcp.Required())), h.HandleSummarize)
	mcpSrv.AddTool(mcp.NewTool("magicskills_get_section", mcp.WithDescription("Retrieves a granular section."), 
		mcp.WithString("name", mcp.Description("The name of the skill"), mcp.Required()),
		mcp.WithString("section", mcp.Description("Section name"))), h.HandleGetSection)
	mcpSrv.AddTool(mcp.NewTool("magicskills_bootstrap", mcp.WithDescription("Extracts a task checklist."), 
		mcp.WithString("name", mcp.Description("The skill name"), mcp.Required())), h.HandleBootstrapTask)
	mcpSrv.AddTool(mcp.NewTool("magicskills_match", mcp.WithDescription("Suggests skills based on intent."), 
		mcp.WithString("intent", mcp.Description("Your goal"), mcp.Required())), h.HandleMatchSkills)
	mcpSrv.AddTool(mcp.NewTool("magicskills_get_logs", mcp.WithDescription("Retrieves internal server logs.")), h.HandleGetLogs)

	mcpSrv.AddResource(mcp.NewResource("magicskills://status", "Skill Status Dashboard",
		mcp.WithResourceDescription("Index health dashboard."), mcp.WithMIMEType("text/markdown")), h.HandleReadResource)

	// Background Watcher
	s.Listen(func() {
		files, _ := s.Discover()
		_ = eng.Ingest(files)
		slog.Info("engine cache refreshed")
	})

	// Graceful Shutdown Goroutine
	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return server.ServeStdio(mcpSrv)
}
