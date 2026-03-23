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
var Version = "2.2.0"

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
	logBuffer := &handler.LogBuffer{}
	setupLogging(logBuffer)

	roots := resolveRoots()
	eng := engine.NewEngine()
	h := &handler.MagicSkillsHandler{
		Engine: eng,
		Logs:   logBuffer,
	}

	// Add manual roots from args
	for _, arg := range flag.Args() {
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			roots = append(roots, arg)
			slog.Info("added manual skill root", "path", arg)
		}
	}

	s, err := scanner.NewScanner(roots)
	if err != nil {
		return fmt.Errorf("scanner init: %w", err)
	}
	defer s.Watcher.Close()

	// Initial Ingestion
	files, err := s.Discover()
	if err != nil {
		slog.Warn("discovery produced errors", "error", err)
	}
	if err := eng.Ingest(files); err != nil {
		slog.Error("initial ingestion failed", "error", err)
	}
	slog.Info("engine ready", "skillsCount", len(eng.Skills), "version", Version, "rootsCount", len(roots))

	// Setup MCP Server
	mcpSrv := server.NewMCPServer("mcp-server-magicskills", Version, server.WithLogging())
	registerTools(mcpSrv, h, s, eng)

	mcpSrv.AddResource(mcp.NewResource("magicskills://status", "Skill Status Dashboard",
		mcp.WithResourceDescription("Index health dashboard."), mcp.WithMIMEType("text/markdown")), h.HandleReadResource)

	// Background Incremental Watcher
	s.Listen(func(path string) {
		if err := eng.IngestSingle(path); err != nil {
			slog.Error("incremental update failed", "path", path, "error", err)
		} else {
			slog.Info("engine cache updated", "path", path)
		}
	}, func(path string) {
		eng.Remove(path)
		slog.Info("engine cache item removed", "path", path)
	})

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received; stopping server")
	}()

	return server.ServeStdio(mcpSrv)
}

func setupLogging(logBuffer *handler.LogBuffer) {
	mw := io.MultiWriter(os.Stderr, logBuffer)
	logger := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
}

func resolveRoots() []string {
	var roots []string
	homePath, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("failed to determine user home directory", "error", err)
	}

	if val := os.Getenv("MAGIC_SKILLS_PATH"); val != "" {
		roots = append(roots, filepath.SplitList(val)...)
	}

	candidates := []string{
		filepath.Join(homePath, ".gemini/skills"),
		filepath.Join(homePath, ".gemini/antigravity/skills"),
		filepath.Join(homePath, ".antigravity/skills"),
		filepath.Join(homePath, ".agents/skills"),
		filepath.Join(homePath, ".agent/skills"),
		filepath.Join(homePath, ".claude/rules"),
		filepath.Join(homePath, ".cursor/rules"),
		filepath.Join(homePath, ".copilot/skills"),
		filepath.Join(homePath, "gitrepos/saxsmith-global-context/.agent/skills"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}

	// Dynamic Workspace Discovery
	roots = append(roots, scanner.FindProjectSkillsRoots()...)

	// Deduplicate roots (Canonicalization)
	seen := make(map[string]struct{})
	unique := make([]string, 0, len(roots))
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			abs = r
		}
		if _, ok := seen[abs]; !ok {
			seen[abs] = struct{}{}
			unique = append(unique, abs)
		}
	}
	return unique
}

func registerTools(mcpSrv *server.MCPServer, h *handler.MagicSkillsHandler, s *scanner.Scanner, eng *engine.Engine) {
	// Tool 1: Index Discovery
	mcpSrv.AddTool(mcp.NewTool("magicskills_list", mcp.WithDescription("Lists all available skills in the authoritative index.")), h.HandleListSkills)

	// Tool 2: Knowledge Retrieval (Consolidated)
	mcpSrv.AddTool(mcp.NewTool("magicskills_get", mcp.WithDescription("Retrieves high-relevance knowledge. Use 'section' to drill down, or leave empty for a dense summary."),
		mcp.WithString("name", mcp.Description("The name of the skill to retrieve"), mcp.Required()),
		mcp.WithString("section", mcp.Description("Optional granular section to retrieve")),
		mcp.WithString("version", mcp.Description("Optional minimum semver bound (e.g. 1.2.0)")),
	), h.HandleGetSkill)

	// Tool 3: Intent-based Discovery
	mcpSrv.AddTool(mcp.NewTool("magicskills_match", mcp.WithDescription("Finds matching skills based on your goal and returns a dense digest immediately."),
		mcp.WithString("intent", mcp.Description("Your goal"), mcp.Required())), h.HandleMatchSkills)

	// Tool 4: Task Management
	mcpSrv.AddTool(mcp.NewTool("magicskills_bootstrap", mcp.WithDescription("Extracts a task checklist from a skill's workflow."),
		mcp.WithString("name", mcp.Description("The skill name"), mcp.Required())), h.HandleBootstrapTask)

	mcpSrv.AddTool(mcp.NewTool("magicskills_validate_deps", mcp.WithDescription("Validate host dependencies (binaries) required by a skill workflow."),
		mcp.WithString("name", mcp.Description("The skill name"), mcp.Required())), h.HandleValidateDeps)

	// Tool 5: Index Management
	mcpSrv.AddTool(mcp.NewTool("magicskills_add_root", mcp.WithDescription("Adds and indexes a new skill directory root."),
		mcp.WithString("path", mcp.Description("Absolute path to index"), mcp.Required())), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := request.GetString("path", "")
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			s.Roots = append(s.Roots, path)
			files, err := s.Discover()
			if err != nil {
				slog.Error("discovery error while adding manual root", "error", err)
				return mcp.NewToolResultError(fmt.Sprintf("discovery error: %v", err)), nil
			}
			if err := eng.Ingest(files); err != nil {
				slog.Error("ingestion error while adding manual root", "error", err)
				return mcp.NewToolResultError(fmt.Sprintf("ingestion error: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Added and indexed: %s", path)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %s", path)), nil
	})
}
