package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}
	for _, tc := range tests {
		got := FormatSize(tc.input)
		if got != tc.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestReadWriteFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello world\nline 2\n"

	if err := WriteFileContent(path, content); err != nil {
		t.Fatalf("WriteFileContent: %v", err)
	}

	got, err := ReadFileContent(path)
	if err != nil {
		t.Fatalf("ReadFileContent: %v", err)
	}
	if got != content {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Overwrite existing file (atomic path).
	newContent := "replaced content"
	if err := WriteFileContent(path, newContent); err != nil {
		t.Fatalf("WriteFileContent overwrite: %v", err)
	}
	got, err = ReadFileContent(path)
	if err != nil {
		t.Fatalf("ReadFileContent after overwrite: %v", err)
	}
	if got != newContent {
		t.Errorf("after overwrite: got %q, want %q", got, newContent)
	}
}

func TestHeadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "head.txt")
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", i+1)
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)

	got, err := HeadFile(path, 5)
	if err != nil {
		t.Fatalf("HeadFile: %v", err)
	}
	gotLines := strings.Split(got, "\n")
	if len(gotLines) != 5 {
		t.Errorf("HeadFile returned %d lines, want 5", len(gotLines))
	}
}

func TestTailFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tail.txt")
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("y", i+1)
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)

	got, err := TailFile(path, 3)
	if err != nil {
		t.Fatalf("TailFile: %v", err)
	}
	gotLines := strings.Split(got, "\n")
	if len(gotLines) != 3 {
		t.Errorf("TailFile returned %d lines, want 3", len(gotLines))
	}
	// Last line should be 20 y's.
	if gotLines[2] != strings.Repeat("y", 20) {
		t.Errorf("last line = %q, want %q", gotLines[2], strings.Repeat("y", 20))
	}
}

func TestApplyFileEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	original := "line one\nline two\nline three\n"
	os.WriteFile(path, []byte(original), 0644)

	diff, err := ApplyFileEdits(path, []FileEdit{
		{OldText: "line two", NewText: "LINE TWO REPLACED"},
	}, false)
	if err != nil {
		t.Fatalf("ApplyFileEdits: %v", err)
	}

	if !strings.Contains(diff, "LINE TWO REPLACED") {
		t.Error("diff should contain the replacement text")
	}

	got, _ := ReadFileContent(path)
	if !strings.Contains(got, "LINE TWO REPLACED") {
		t.Error("file should contain the replacement text")
	}
}

func TestApplyFileEditsDryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dryrun.txt")
	original := "keep this\nchange this\n"
	os.WriteFile(path, []byte(original), 0644)

	_, err := ApplyFileEdits(path, []FileEdit{
		{OldText: "change this", NewText: "CHANGED"},
	}, true)
	if err != nil {
		t.Fatalf("ApplyFileEdits dry run: %v", err)
	}

	got, _ := ReadFileContent(path)
	if strings.Contains(got, "CHANGED") {
		t.Error("dry run should not modify the file")
	}
}

func TestBuildDirectoryTree(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("r"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "child.txt"), []byte("c"), 0644)

	tree, err := BuildDirectoryTree(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("BuildDirectoryTree: %v", err)
	}
	if len(tree) != 2 {
		t.Errorf("expected 2 top-level entries, got %d", len(tree))
	}
}

func TestSearchFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "foo.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "bar.go"), []byte("y"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "readme.md"), []byte("z"), 0644)

	results, err := SearchFiles(context.Background(), dir, "**/*.go", nil)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 .go files, got %d: %v", len(results), results)
	}
}

func TestGetFileStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stat.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	info, err := GetFileStats(path)
	if err != nil {
		t.Fatalf("GetFileStats: %v", err)
	}
	if info.Size != 5 {
		t.Errorf("size = %d, want 5", info.Size)
	}
	if !info.IsFile {
		t.Error("expected isFile to be true")
	}
}

func TestMIMEType(t *testing.T) {
	if got := MIMEType(".png"); got != "image/png" {
		t.Errorf("MIMEType(.png) = %q", got)
	}
	if got := MIMEType(".xyz"); got != "application/octet-stream" {
		t.Errorf("MIMEType(.xyz) = %q", got)
	}
}
