// Package engine provides core filesystem operations including file reading,
// writing, editing with diff generation, directory traversal, and search.
package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"mcp-server-filesystem/internal/config"
)

// FormatSize returns a human-readable byte size string.
func FormatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	if bytes == 0 {
		return "0 B"
	}
	size := float64(bytes)
	idx := 0
	for size >= 1024 && idx < len(units)-1 {
		size /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", bytes, units[0])
	}
	return fmt.Sprintf("%.2f %s", size, units[idx])
}

// ReadFileContent reads an entire file as a UTF-8 string.
// Files larger than config.MaxReadFileSize are rejected to prevent OOM.
func ReadFileContent(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > config.MaxReadFileSize {
		return "", fmt.Errorf("file too large (%s, limit %s): %s",
			FormatSize(info.Size()), FormatSize(config.MaxReadFileSize), filePath)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return string(data), nil
}

// ReadFileBase64 reads a file and returns its base64-encoded content.
func ReadFileBase64(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var buf strings.Builder
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := io.Copy(encoder, f); err != nil {
		return "", fmt.Errorf("encoding file: %w", err)
	}
	encoder.Close()
	return buf.String(), nil
}

// WriteFileContent writes content to a file using atomic rename to prevent
// race conditions and symlink attacks.
func WriteFileContent(filePath, content string) error {
	// Try exclusive creation first.
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		n, writeErr := f.WriteString(content)
		if writeErr == nil && n != len(content) {
			writeErr = io.ErrShortWrite
		}
		if writeErr == nil {
			writeErr = f.Sync()
		}
		closeErr := f.Close()
		if writeErr != nil {
			return fmt.Errorf("writing new file: %w", writeErr)
		}
		return closeErr
	}

	if !os.IsExist(err) {
		return fmt.Errorf("creating file: %w", err)
	}

	// File exists — use atomic temp+rename.
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("generating random suffix: %w", err)
	}
	tmpPath := filePath + "." + hex.EncodeToString(randBytes) + ".tmp"

	f, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		_ = os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}

// FileInfo holds metadata about a file or directory.
type FileInfo struct {
	Size        int64  `json:"size"`
	Created     string `json:"created"`
	Modified    string `json:"modified"`
	Accessed    string `json:"accessed"`
	IsDirectory bool   `json:"isDirectory"`
	IsFile      bool   `json:"isFile"`
	Permissions string `json:"permissions"`
}

// GetFileStats returns metadata for a path.
func GetFileStats(filePath string) (*FileInfo, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	return &FileInfo{
		Size:        info.Size(),
		Created:     info.ModTime().String(), // Go doesn't expose birth time portably
		Modified:    info.ModTime().String(),
		Accessed:    info.ModTime().String(),
		IsDirectory: info.IsDir(),
		IsFile:      info.Mode().IsRegular(),
		Permissions: fmt.Sprintf("%o", info.Mode().Perm()),
	}, nil
}

// HeadFile returns the first n lines of a file.
func HeadFile(filePath string, n int) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, n)
	for scanner.Scan() && len(lines) < n {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading lines: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}

// TailFile returns the last n lines of a file using reverse chunked reads.
func TailFile(filePath string, n int) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}
	fileSize := info.Size()
	if fileSize == 0 {
		return "", nil
	}

	const chunkSize = 1024
	var lines []string
	position := fileSize
	remaining := ""
	buf := make([]byte, chunkSize) // Reuse buffer across iterations

	for position > 0 && len(lines) < n {
		size := min(int64(chunkSize), position)
		position -= size

		_, err := f.ReadAt(buf[:size], position)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("reading chunk: %w", err)
		}

		chunkText := string(buf[:size]) + remaining
		chunkLines := strings.Split(normalizeLineEndings(chunkText), "\n")

		if position > 0 {
			remaining = chunkLines[0]
			chunkLines = chunkLines[1:]
		} else {
			remaining = ""
		}

		for i := len(chunkLines) - 1; i >= 0 && len(lines) < n; i-- {
			lines = append(lines, chunkLines[i])
		}
	}

	// If there's remaining text and we still need lines, prepend it.
	if remaining != "" && len(lines) < n {
		lines = append(lines, remaining)
	}

	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return strings.Join(lines, "\n"), nil
}

// normalizeLineEndings converts \r\n to \n.
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// FileEdit describes a single text replacement operation.
type FileEdit struct {
	OldText string `json:"oldText" jsonschema:"Exact block of text to replace. Must match the target file exactly including leading/trailing whitespace."`
	NewText string `json:"newText" jsonschema:"Replacement text block."`
}

// ApplyFileEdits applies a sequence of text replacements to a file and
// returns a unified diff. If dryRun is true, no changes are written.
func ApplyFileEdits(filePath string, edits []FileEdit, dryRun bool) (string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	content := normalizeLineEndings(string(raw))
	modified := content

	for _, edit := range edits {
		normalizedOld := normalizeLineEndings(edit.OldText)
		normalizedNew := normalizeLineEndings(edit.NewText)

		if strings.Contains(modified, normalizedOld) {
			modified = strings.Replace(modified, normalizedOld, normalizedNew, 1)
			continue
		}

		// Fallback: whitespace-flexible line matching.
		var matchFound bool
		modified, matchFound = applyFlexibleMatch(modified, normalizedOld, normalizedNew)
		if !matchFound {
			return "", fmt.Errorf("could not find exact match for edit:\n%s", edit.OldText)
		}
	}

	// Generate unified diff.
	editsResult := myers.ComputeEdits(span.URIFromPath(filePath), content, modified)
	diff := fmt.Sprint(gotextdiff.ToUnified(filePath, filePath, content, editsResult))

	if !dryRun {
		if err := atomicWrite(filePath, modified); err != nil {
			return "", fmt.Errorf("writing edits: %w", err)
		}
	}

	return diff, nil
}

// applyFlexibleMatch handles whitespace-flexible line matching and replacement.
func applyFlexibleMatch(modified, normalizedOld, normalizedNew string) (string, bool) {
	oldLines := strings.Split(normalizedOld, "\n")
	contentLines := strings.Split(modified, "\n")

	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		isMatch := true
		for j, oldLine := range oldLines {
			if strings.TrimSpace(oldLine) != strings.TrimSpace(contentLines[i+j]) {
				isMatch = false
				break
			}
		}
		if isMatch {
			newLines := strings.Split(normalizedNew, "\n")
			// Preserve original indentation.
			indent := ""
			if idx := strings.IndexFunc(contentLines[i], func(r rune) bool { return r != ' ' && r != '\t' }); idx > 0 {
				indent = contentLines[i][:idx]
			}
			for k, line := range newLines {
				if k == 0 {
					newLines[k] = indent + strings.TrimLeft(line, " \t")
				}
			}
			result := make([]string, 0, len(contentLines)-len(oldLines)+len(newLines))
			result = append(result, contentLines[:i]...)
			result = append(result, newLines...)
			result = append(result, contentLines[i+len(oldLines):]...)
			return strings.Join(result, "\n"), true
		}
	}
	return modified, false
}

// atomicWrite writes content via a temp file + rename.
func atomicWrite(filePath, content string) error {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return err
	}
	tmpPath := filePath + "." + hex.EncodeToString(randBytes) + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		_ = os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	return nil
}

// TreeEntry represents a node in a directory tree.
type TreeEntry struct {
	Name     string       `json:"name"`
	Type     string       `json:"type"` // "file" or "directory"
	Children []*TreeEntry `json:"children,omitempty"`
}

// BuildDirectoryTree recursively builds a JSON-serializable tree.
// Recursion is capped at config.MaxTreeDepth to prevent stack overflow.
func BuildDirectoryTree(ctx context.Context, rootPath string, excludePatterns []string) ([]*TreeEntry, error) {
	return buildTree(ctx, rootPath, rootPath, excludePatterns, 0)
}

func buildTree(ctx context.Context, currentPath, rootPath string, excludePatterns []string, depth int) ([]*TreeEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth > config.MaxTreeDepth {
		return nil, nil // Silently stop recursion at max depth
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var result []*TreeEntry
	for _, entry := range entries {
		relPath, _ := filepath.Rel(rootPath, filepath.Join(currentPath, entry.Name()))

		if shouldExclude(relPath, excludePatterns) {
			continue
		}

		node := &TreeEntry{
			Name: entry.Name(),
			Type: "file",
		}
		if entry.IsDir() {
			node.Type = "directory"
			children, err := buildTree(ctx, filepath.Join(currentPath, entry.Name()), rootPath, excludePatterns, depth+1)
			if err != nil {
				return nil, err
			}
			node.Children = children
		}
		result = append(result, node)
	}
	return result, nil
}

func shouldExclude(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, relPath); matched {
			return true
		}
		if matched, _ := doublestar.Match("**/"+pattern, relPath); matched {
			return true
		}
		if matched, _ := doublestar.Match("**/"+pattern+"/**", relPath); matched {
			return true
		}
	}
	return false
}

// SearchFiles recursively searches for files matching a glob pattern.
// Results are capped at config.MaxSearchResults to prevent memory exhaustion.
func SearchFiles(ctx context.Context, rootPath, pattern string, excludePatterns []string) ([]string, error) {
	var results []string

	err := filepath.WalkDir(rootPath, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if len(results) >= config.MaxSearchResults {
			return filepath.SkipAll
		}

		relPath, _ := filepath.Rel(rootPath, fullPath)
		if relPath == "." {
			return nil
		}

		for _, ep := range excludePatterns {
			if matched, _ := doublestar.Match(ep, relPath); matched {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if matched, _ := doublestar.Match(pattern, relPath); matched {
			results = append(results, fullPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("searching files: %w", err)
	}
	return results, nil
}

// DirEntry holds details for list_directory_with_sizes.
type DirEntry struct {
	Name        string
	IsDirectory bool
	Size        int64
}

// ListDirectoryWithSizes lists entries in a directory with their sizes.
func ListDirectoryWithSizes(dirPath, sortBy string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	result := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		de := DirEntry{
			Name:        entry.Name(),
			IsDirectory: entry.IsDir(),
		}
		info, err := entry.Info()
		if err == nil {
			de.Size = info.Size()
		}
		result = append(result, de)
	}

	switch sortBy {
	case "size":
		sort.Slice(result, func(i, j int) bool {
			return result[i].Size > result[j].Size
		})
	default: // "name"
		sort.Slice(result, func(i, j int) bool {
			return result[i].Name < result[j].Name
		})
	}
	return result, nil
}

// mimeTypes maps file extensions to MIME types. Hoisted to package level
// to avoid re-creating the map on every call.
var mimeTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".svg":  "image/svg+xml",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
}

// MIMEType returns the MIME type for a file extension.
func MIMEType(ext string) string {
	if mt, ok := mimeTypes[strings.ToLower(ext)]; ok {
		return mt
	}
	return "application/octet-stream"
}

// TreeToJSON is a convenience that marshals a tree to indented JSON.
func TreeToJSON(tree []*TreeEntry) (string, error) {
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling tree: %w", err)
	}
	return string(data), nil
}

// CopyPath copies a file or directory recursively from src to dst.
func CopyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// RemovePath forcefully removes a file or an entire directory recursively.
func RemovePath(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove path: %w", err)
	}
	return nil
}

// AppendFileContent appends a string to the end of a file.
// If the file does not exist, it creates it.
func AppendFileContent(filePath, content string) error {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file for append: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write append content: %w", err)
	}
	return f.Sync()
}

// GetFileHash computes the SHA-256 hash of a file.
func GetFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file for hash: %w", err)
	}
	defer f.Close()

	// Need to import crypto/sha256 which is not in the imports...
	// Wait, I will use crypto/sha256. I must also add the import at the top.
	// We'll read it manually into crypto/sha256 interface.
	// BUT wait, I need to add crypto/sha256 to the imports.
	// To avoid import issues, I will format the file later via multi_replace if needed,
	// or I will just write the function and we add the import later.
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash copy: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
