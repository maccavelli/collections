package filesystem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-filesystem/internal/pathutil"
)

func TestFilesystemTools_Register(t *testing.T) {
	dir := t.TempDir()
	pm := pathutil.NewManager([]string{dir})
	Register(pm)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	(&ReadTextFileTool{PM: pm}).Register(srv)
	(&ReadTextFileTool{PM: pm}).Handle(ctx, req, ReadTextFileInput{Path: dir})

	(&ReadMediaFileTool{PM: pm}).Register(srv)
	(&ReadMediaFileTool{PM: pm}).Handle(ctx, req, ReadMediaFileInput{Path: dir})

	(&ReadMultipleFilesTool{PM: pm}).Register(srv)
	(&ReadMultipleFilesTool{PM: pm}).Handle(ctx, req, ReadMultipleFilesInput{Paths: []string{dir}})

	(&WriteFileTool{PM: pm}).Register(srv)
	(&WriteFileTool{PM: pm}).Handle(ctx, req, WriteFileInput{Path: filepath.Join(dir, "test.txt")})

	(&EditFileTool{PM: pm}).Register(srv)
	(&EditFileTool{PM: pm}).Handle(ctx, req, EditFileInput{Path: dir})

	(&CreateDirectoryTool{PM: pm}).Register(srv)
	(&CreateDirectoryTool{PM: pm}).Handle(ctx, req, SinglePathInput{Path: dir})

	(&ListDirectoryTool{PM: pm}).Register(srv)
	(&ListDirectoryTool{PM: pm}).Handle(ctx, req, SinglePathInput{Path: dir})

	(&ListDirectoryWithSizesTool{PM: pm}).Register(srv)
	(&ListDirectoryWithSizesTool{PM: pm}).Handle(ctx, req, ListDirectoryWithSizesInput{Path: dir})

	(&DirectoryTreeTool{PM: pm}).Register(srv)
	(&DirectoryTreeTool{PM: pm}).Handle(ctx, req, DirectoryTreeInput{Path: dir})

	(&MoveFileTool{PM: pm}).Register(srv)
	(&MoveFileTool{PM: pm}).Handle(ctx, req, MoveFileInput{Source: dir, Destination: dir})

	(&SearchFilesTool{PM: pm}).Register(srv)
	(&SearchFilesTool{PM: pm}).Handle(ctx, req, SearchFilesInput{Path: string([]rune(dir)[:1])}) // searching root tempdir takes way too long maybe?

	(&GetFileInfoTool{PM: pm}).Register(srv)
	(&GetFileInfoTool{PM: pm}).Handle(ctx, req, SinglePathInput{Path: dir})

	(&ListAllowedDirectoriesTool{PM: pm}).Register(srv)
	(&ListAllowedDirectoriesTool{PM: pm}).Handle(ctx, req, EmptyInput{})
}
