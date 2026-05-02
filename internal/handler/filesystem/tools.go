// Package filesystem registers all filesystem MCP tools.
package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-filesystem/internal/engine"
	"mcp-server-filesystem/internal/pathutil"
	"mcp-server-filesystem/internal/registry"
	"mcp-server-filesystem/internal/util"
)

// --- Read Text File ---

// ReadTextFileTool reads a text file with optional head/tail.
type ReadTextFileTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ReadTextFileTool) Name() string { return "read_text_file" }

// Register exposes the tool schema to the MCP protocol.
func (t *ReadTextFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: File Inspection] Read, view, open, cat, or print a text file. Supports optional 'head' (first N lines) or 'tail' (last N lines) parameters natively. Keywords: read-content, view-code, cat, string-data, head, tail",
	}, t.Handle)
}

// ReadTextFileInput is the input schema for read_text_file.
type ReadTextFileInput struct {
	Path string `json:"path" jsonschema:"File path to read"`
	Tail *int   `json:"tail,omitempty" jsonschema:"If provided, returns only the last N lines"`
	Head *int   `json:"head,omitempty" jsonschema:"If provided, returns only the first N lines"`
}

// Handle executes the filesystem tool logic.
func (t *ReadTextFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ReadTextFileInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	if input.Head != nil && input.Tail != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("cannot specify both head and tail parameters simultaneously"))
		return res, nil, nil
	}

	var content string
	switch {
	case input.Tail != nil:
		content, err = engine.TailFile(validPath, *input.Tail)
	case input.Head != nil:
		content, err = engine.HeadFile(validPath, *input.Head)
	default:
		content, err = engine.ReadFileContent(validPath)
	}
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": content}, nil
}

// --- Read Media File ---

// ReadMediaFileTool reads a binary file and returns base64.
type ReadMediaFileTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ReadMediaFileTool) Name() string { return "read_media_file" }

// Register exposes the tool schema to the MCP protocol.
func (t *ReadMediaFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Multimedia Extraction] Read, view, or open an image or audio file. Returns base64 encoded data and its MIME type securely. Keywords: binary-data, base64, audio, pictures, images, visual, assets",
	}, t.Handle)
}

// ReadMediaFileInput is the schema for read_media_file.
type ReadMediaFileInput struct {
	Path string `json:"path" jsonschema:"File path to read"`
}

// MediaResult represents the JSON response for a media file.
type MediaResult struct {
	Content []MediaContent `json:"content"`
}

// MediaContent holds the base64 media payload.
type MediaContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// Handle executes the filesystem tool logic.
func (t *ReadMediaFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ReadMediaFileInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	ext := strings.ToLower(filepath.Ext(validPath))
	mimeType := engine.MIMEType(ext)
	data, err := engine.ReadFileBase64(validPath)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	mediaType := "blob"
	if strings.HasPrefix(mimeType, "image/") {
		mediaType = "image"
	} else if strings.HasPrefix(mimeType, "audio/") {
		mediaType = "audio"
	}

	return &mcp.CallToolResult{}, MediaResult{
		Content: []MediaContent{
			{Type: mediaType, Data: data, MimeType: mimeType},
		},
	}, nil
}

// --- Read Multiple Files ---

// ReadMultipleFilesTool reads several files at once.
type ReadMultipleFilesTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ReadMultipleFilesTool) Name() string { return "read_multiple_files" }

// Register exposes the tool schema to the MCP protocol.
func (t *ReadMultipleFilesTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Batch Inspection] Read, view, or open multiple text files simultaneously. Failed reads are omitted gracefully natively. Keywords: multi-read, batch-fetch, array-read, bulk-content",
	}, t.Handle)
}

// ReadMultipleFilesInput is the schema for read_multiple_files.
type ReadMultipleFilesInput struct {
	Paths []string `json:"paths" jsonschema:"Array of file paths to read" jsonschema_extras:"minItems=1"`
}

// Handle executes the filesystem tool logic.
func (t *ReadMultipleFilesTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ReadMultipleFilesInput) (*mcp.CallToolResult, any, error) {
	parts := make([]string, 0, len(input.Paths))
	for _, p := range input.Paths {
		validPath, err := t.PM.ValidatePath(p)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s: Error - %v", p, err))
			continue
		}
		content, err := engine.ReadFileContent(validPath)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s: Error - %v", p, err))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:\n%s\n", p, content))
	}
	text := strings.Join(parts, "\n---\n")
	return &mcp.CallToolResult{}, map[string]string{"content": text}, nil
}

// --- Write File ---

// WriteFileTool creates or overwrites a file.
type WriteFileTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *WriteFileTool) Name() string { return "write_file" }

// Register exposes the tool schema to the MCP protocol.
func (t *WriteFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Destructive Generation] Completely overwrite or create a file natively. CAUTION: Destructive operation dropping previous state natively. Keywords: create-new, overwrite-data, truncate, write-content, initialize",
	}, t.Handle)
}

// WriteFileInput is the schema for write_file.
type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"File path to write"`
	Content string `json:"content" jsonschema:"Content to write"`
}

// Handle executes the filesystem tool logic.
func (t *WriteFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input WriteFileInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := engine.WriteFileContent(validPath, input.Content); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully wrote to %s", input.Path)}, nil
}

// --- Edit File ---

// EditFileTool applies text edits and returns a diff.
type EditFileTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *EditFileTool) Name() string { return "edit_file" }

// Register exposes the tool schema to the MCP protocol.
func (t *EditFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Precise Patching] Apply exact line-based text edits, modify, replace, change, or rewrite a file natively. Returns a git-style diff globally. Keywords: modify-code, line-edit, string-replace, diff-patch, rewrite",
	}, t.Handle)
}

// EditFileInput is the schema for edit_file.
type EditFileInput struct {
	Path   string            `json:"path" jsonschema:"File path to edit"`
	Edits  []engine.FileEdit `json:"edits" jsonschema:"Array of edit operations"`
	DryRun bool              `json:"dryRun,omitzero" jsonschema:"Preview changes using git-style diff format"`
}

// Handle executes the filesystem tool logic.
func (t *EditFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input EditFileInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	diff, err := engine.ApplyFileEdits(validPath, input.Edits, input.DryRun)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": diff}, nil
}

// --- Create Directory ---

// CreateDirectoryTool creates directories recursively.
type CreateDirectoryTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *CreateDirectoryTool) Name() string { return "create_directory" }

// Register exposes the tool schema to the MCP protocol.
func (t *CreateDirectoryTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Structural Allocation] Create nested directories (mkdir -p) recursively establishing physical bounds. Keywords: mkdir, new-folder, path-creation, allocate-dir",
	}, t.Handle)
}

// SinglePathInput is a schema for tools that take only a path.
type SinglePathInput struct {
	Path string `json:"path" jsonschema:"Directory or file path"`
}

// Handle executes the filesystem tool logic.
func (t *CreateDirectoryTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SinglePathInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := os.MkdirAll(validPath, 0755); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully created directory %s", input.Path)}, nil
}

// --- List Directory ---

// ListDirectoryTool lists files and directories.
type ListDirectoryTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ListDirectoryTool) Name() string { return "list_directory" }

// Register exposes the tool schema to the MCP protocol.
func (t *ListDirectoryTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Basic Enumeration] List files and directories natively in a path. Outputs are prefixed with [FILE] or [DIR] explicitly. Keywords: ls, show-files, directory-contents, map-folder",
	}, t.Handle)
}

// Handle executes the filesystem tool logic.
func (t *ListDirectoryTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SinglePathInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	entries, err := os.ReadDir(validPath)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		prefix := "[FILE]"
		if entry.IsDir() {
			prefix = "[DIR]"
		}
		lines = append(lines, fmt.Sprintf("%s %s", prefix, entry.Name()))
	}
	return &mcp.CallToolResult{}, map[string]string{"content": strings.Join(lines, "\n")}, nil
}

// --- List Directory with Sizes ---

// ListDirectoryWithSizesTool lists with size info.
type ListDirectoryWithSizesTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ListDirectoryWithSizesTool) Name() string { return "list_directory_with_sizes" }

// Register exposes the tool schema to the MCP protocol.
func (t *ListDirectoryWithSizesTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Footprint Enumeration] List files and directories natively with exact sizes. Results can be sorted by 'name' or 'size' globally. Keywords: ls-la, file-sizes, heavy-files, sorted-directory",
	}, t.Handle)
}

// ListDirectoryWithSizesInput is the schema.
type ListDirectoryWithSizesInput struct {
	Path   string `json:"path" jsonschema:"Directory path"`
	SortBy string `json:"sortBy,omitzero" jsonschema:"Sort entries by 'name' or 'size',enum=name|size"`
}

// Handle executes the filesystem tool logic.
func (t *ListDirectoryWithSizesTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input ListDirectoryWithSizesInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	sortBy := input.SortBy
	if sortBy == "" {
		sortBy = "name"
	}

	entries, err := engine.ListDirectoryWithSizes(validPath, sortBy)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	var sb strings.Builder
	var totalFiles, totalDirs int
	var totalSize int64
	for _, e := range entries {
		prefix := "[FILE]"
		if e.IsDirectory {
			prefix = "[DIR]"
			totalDirs++
		} else {
			totalFiles++
			totalSize += e.Size
		}
		sizeStr := ""
		if !e.IsDirectory {
			sizeStr = fmt.Sprintf("%10s", engine.FormatSize(e.Size))
		}
		sb.WriteString(fmt.Sprintf("%s %-30s %s\n", prefix, e.Name, sizeStr))
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d files, %d directories\n", totalFiles, totalDirs))
	sb.WriteString(fmt.Sprintf("Combined size: %s", engine.FormatSize(totalSize)))

	return &mcp.CallToolResult{}, map[string]string{"content": sb.String()}, nil
}

// --- Directory Tree ---

// DirectoryTreeTool builds a recursive JSON tree.
type DirectoryTreeTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *DirectoryTreeTool) Name() string { return "directory_tree" }

// Register exposes the tool schema to the MCP protocol.
func (t *DirectoryTreeTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Topological Mapping] Generate a recursive JSON directory tree natively. Supports explicit exclusion glob patterns. Keywords: tree, recursive-map, project-structure, nested-folders",
	}, t.Handle)
}

// DirectoryTreeInput is the schema.
type DirectoryTreeInput struct {
	Path            string   `json:"path" jsonschema:"Root directory path"`
	ExcludePatterns []string `json:"excludePatterns,omitempty" jsonschema:"Glob patterns to exclude"`
}

// Handle executes the filesystem tool logic.
func (t *DirectoryTreeTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input DirectoryTreeInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	tree, err := engine.BuildDirectoryTree(ctx, validPath, input.ExcludePatterns)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	jsonStr, err := engine.TreeToJSON(tree)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	return &mcp.CallToolResult{}, map[string]string{"content": jsonStr}, nil
}

// --- Move File ---

// MoveFileTool moves or renames files.
type MoveFileTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *MoveFileTool) Name() string { return "move_file" }

// Register exposes the tool schema to the MCP protocol.
func (t *MoveFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Path Reallocation] Move or rename files and directories atomically natively. Keywords: mv, rename, migrate, shift-location",
	}, t.Handle)
}

// MoveFileInput is the schema for move_file.
type MoveFileInput struct {
	Source      string `json:"source" jsonschema:"Source path"`
	Destination string `json:"destination" jsonschema:"Destination path"`
}

// Handle executes the filesystem tool logic.
func (t *MoveFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input MoveFileInput) (*mcp.CallToolResult, any, error) {
	validSrc, err := t.PM.ValidatePath(input.Source)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	validDst, err := t.PM.ValidatePath(input.Destination)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := os.Rename(validSrc, validDst); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully moved %s to %s", input.Source, input.Destination)}, nil
}

// --- Search Files ---

// SearchFilesTool recursively searches with glob.
type SearchFilesTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *SearchFilesTool) Name() string { return "search_files" }

// Register exposes the tool schema to the MCP protocol.
func (t *SearchFilesTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Pattern Discovery] Search, find, locate, or glob for files matching a string pattern recursively natively. Keywords: find, glob, recursive-search, locate, pattern-match",
	}, t.Handle)
}

// SearchFilesInput is the schema.
type SearchFilesInput struct {
	Path            string   `json:"path" jsonschema:"Root search path"`
	Pattern         string   `json:"pattern" jsonschema:"Glob pattern to match"`
	ExcludePatterns []string `json:"excludePatterns,omitempty" jsonschema:"Glob patterns to exclude"`
}

// Handle executes the filesystem tool logic.
func (t *SearchFilesTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SearchFilesInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	results, err := engine.SearchFiles(ctx, validPath, input.Pattern, input.ExcludePatterns)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}

	text := "No matches found"
	if len(results) > 0 {
		text = strings.Join(results, "\n")
	}
	return &mcp.CallToolResult{}, map[string]string{"content": text}, nil
}

// --- Get File Info ---

// GetFileInfoTool returns file metadata.
type GetFileInfoTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *GetFileInfoTool) Name() string { return "get_file_info" }

// Register exposes the tool schema to the MCP protocol.
func (t *GetFileInfoTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Metadata Extraction] Retrieve structural metadata for a file or directory natively (size, modified time, permissions, type). Keywords: stat, file-metadata, permissions, timestamps, attributes",
	}, t.Handle)
}

// Handle executes the filesystem tool logic.
func (t *GetFileInfoTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SinglePathInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	info, err := engine.GetFileStats(validPath)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	lines := []string{
		fmt.Sprintf("size: %d", info.Size),
		fmt.Sprintf("created: %s", info.Created),
		fmt.Sprintf("modified: %s", info.Modified),
		fmt.Sprintf("accessed: %s", info.Accessed),
		fmt.Sprintf("isDirectory: %t", info.IsDirectory),
		fmt.Sprintf("isFile: %t", info.IsFile),
		fmt.Sprintf("permissions: %s", info.Permissions),
	}
	return &mcp.CallToolResult{}, map[string]string{"content": strings.Join(lines, "\n")}, nil
}

// --- List Allowed Directories ---

// ListAllowedDirectoriesTool lists the allowed dirs.
type ListAllowedDirectoriesTool struct {
	PM *pathutil.Manager
}

// Name returns the unique tool identifier.
func (t *ListAllowedDirectoriesTool) Name() string { return "list_allowed_directories" }

// Register exposes the tool schema to the MCP protocol.
func (t *ListAllowedDirectoriesTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Boundary Verification] List all root workspace bounds this OS server is permitted to access natively. Keywords: allowed-paths, permissions-check, root-access, security-bounds",
	}, t.Handle)
}

// EmptyInput requires no parameters.
type EmptyInput struct{}

// Handle executes the filesystem tool logic.
func (t *ListAllowedDirectoriesTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
	dirs := t.PM.Allowed()
	text := "Allowed directories:\n" + strings.Join(dirs, "\n")
	return &mcp.CallToolResult{}, map[string]string{"content": text}, nil
}

// --- Copy Path ---

type CopyPathTool struct {
	PM *pathutil.Manager
}

func (t *CopyPathTool) Name() string { return "copy_path" }

func (t *CopyPathTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Replication] Copy a file or directory recursively securely natively. Keywords: cp, duplicate, clone, replicate-data",
	}, t.Handle)
}

type CopyPathInput struct {
	Source      string `json:"source" jsonschema:"Source path to copy"`
	Destination string `json:"destination" jsonschema:"Destination path"`
}

func (t *CopyPathTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input CopyPathInput) (*mcp.CallToolResult, any, error) {
	validSrc, err := t.PM.ValidatePath(input.Source)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	validDst, err := t.PM.ValidatePath(input.Destination)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := engine.CopyPath(validSrc, validDst); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("copy failed: %w", err))
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully copied %s to %s", input.Source, input.Destination)}, nil
}

// --- Remove Path ---

type RemovePathTool struct {
	PM *pathutil.Manager
}

func (t *RemovePathTool) Name() string { return "remove_path" }

func (t *RemovePathTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Destructive Eradication] Forcefully delete a file or directory recursively natively. CAUTION: Destructive string override natively. Keywords: rm, delete, wipe, destroy, erase-path",
	}, t.Handle)
}

func (t *RemovePathTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SinglePathInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := engine.RemovePath(validPath); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("remove failed: %w", err))
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully removed %s", input.Path)}, nil
}

// --- Append File ---

type AppendFileTool struct {
	PM *pathutil.Manager
}

func (t *AppendFileTool) Name() string { return "append_file" }

func (t *AppendFileTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Sequential Addition] Append text to the end of a file securely. Creates the file if it does not exist. Keywords: append-text, add-to-end, insert-bottom, concatenate",
	}, t.Handle)
}

func (t *AppendFileTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input WriteFileInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	if err := engine.AppendFileContent(validPath, input.Content); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("append failed: %w", err))
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("Successfully appended to %s", input.Path)}, nil
}

// --- Get File Hash ---

type GetFileHashTool struct {
	PM *pathutil.Manager
}

func (t *GetFileHashTool) Name() string { return "get_file_hash" }

func (t *GetFileHashTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[DIRECTIVE: Cryptographic Integrity] Calculate the absolute SHA-256 hash of a file for integrity verification natively. Keywords: sha256, checksum, fingerprint, verify-hash, integrity-check",
	}, t.Handle)
}

func (t *GetFileHashTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input SinglePathInput) (*mcp.CallToolResult, any, error) {
	validPath, err := t.PM.ValidatePath(input.Path)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	hash, err := engine.GetFileHash(validPath)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("hash computation failed: %w", err))
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, map[string]string{"content": fmt.Sprintf("SHA-256 for %s:\\n%s", input.Path, hash)}, nil
}

// Register adds all filesystem tools to the global registry.
func Register(pm *pathutil.Manager) {
	registry.Global.Register(&ReadTextFileTool{PM: pm})
	registry.Global.Register(&ReadMediaFileTool{PM: pm})
	registry.Global.Register(&ReadMultipleFilesTool{PM: pm})
	registry.Global.Register(&WriteFileTool{PM: pm})
	registry.Global.Register(&EditFileTool{PM: pm})
	registry.Global.Register(&CreateDirectoryTool{PM: pm})
	registry.Global.Register(&ListDirectoryTool{PM: pm})
	registry.Global.Register(&ListDirectoryWithSizesTool{PM: pm})
	registry.Global.Register(&DirectoryTreeTool{PM: pm})
	registry.Global.Register(&MoveFileTool{PM: pm})
	registry.Global.Register(&SearchFilesTool{PM: pm})
	registry.Global.Register(&GetFileInfoTool{PM: pm})
	registry.Global.Register(&ListAllowedDirectoriesTool{PM: pm})
	registry.Global.Register(&CopyPathTool{PM: pm})
	registry.Global.Register(&RemovePathTool{PM: pm})
	registry.Global.Register(&AppendFileTool{PM: pm})
	registry.Global.Register(&GetFileHashTool{PM: pm})

}
